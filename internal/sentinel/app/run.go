// internal/sentinel/app/run.go
package app

import (
	"context"
	"log"
	"time"

	"github.com/gnolang/val-companion/internal/sentinel/config"
	"github.com/gnolang/val-companion/internal/sentinel/rpc"
	"github.com/gnolang/val-companion/internal/sentinel/sender"
	"github.com/gnolang/val-companion/pkg/protocol"
)

const (
	rpcBufferSize   = 100
	maxSendAttempts = 5
	initialBackoff  = time.Second
)

// Run starts all enabled collectors and drains their output to the sender.
// It blocks until ctx is cancelled.
func Run(ctx context.Context, cfg *config.Config) {
	s := sender.New(cfg.Server.URL, cfg.Server.Token)
	rpcOut := make(chan protocol.RPCPayload, rpcBufferSize)
	buf := sender.NewBuffer[protocol.RPCPayload](rpcBufferSize)

	if cfg.RPC.Enabled {
		client := rpc.NewClient(cfg.RPC.RPCURL)
		collector := rpc.NewCollector(
			client,
			cfg.RPC.PollInterval.Duration,
			cfg.RPC.DumpConsensusStateInterval.Duration,
			rpcOut,
			log.Printf,
		)
		go func() {
			if err := collector.Run(ctx); err != nil && ctx.Err() == nil {
				log.Printf("rpc collector stopped: %v", err)
			}
		}()
	}

	// Drain RPC payloads from channel into buffer, then send.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case p := <-rpcOut:
				if dropped := buf.Push(p); dropped {
					log.Printf("rpc buffer full: oldest payload dropped")
				}
			}
		}
	}()

	// Flush buffer to watchtower on each poll interval.
	ticker := time.NewTicker(cfg.RPC.PollInterval.Duration)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			flush(ctx, s, buf)
		}
	}
}

func flush(ctx context.Context, s *sender.Sender, buf *sender.Buffer[protocol.RPCPayload]) {
	items := buf.Drain()
	for _, p := range items {
		if err := s.SendWithRetry(ctx, "/rpc", p, maxSendAttempts, initialBackoff); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("send rpc payload: %v", err)
		}
	}
}
