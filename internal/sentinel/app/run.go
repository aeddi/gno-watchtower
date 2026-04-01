// internal/sentinel/app/run.go
package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/gnolang/val-companion/internal/sentinel/config"
	"github.com/gnolang/val-companion/internal/sentinel/rpc"
	"github.com/gnolang/val-companion/internal/sentinel/sender"
	"github.com/gnolang/val-companion/internal/sentinel/stats"
	"github.com/gnolang/val-companion/pkg/protocol"
)

const (
	rpcBufferSize   = 100
	maxSendAttempts = 5
	initialBackoff  = time.Second
	statsInterval   = time.Minute
)

// Run starts all enabled collectors and drains their output to the sender.
// It blocks until ctx is cancelled.
func Run(ctx context.Context, cfg *config.Config, log *slog.Logger) {
	appLog := log.With("component", "app")
	st := stats.New()

	s := sender.New(cfg.Server.URL, cfg.Server.Token)
	rpcOut := make(chan protocol.RPCPayload, rpcBufferSize)
	buf := sender.NewBuffer[protocol.RPCPayload](rpcBufferSize)

	if !cfg.RPC.Enabled {
		<-ctx.Done()
		return
	}

	client := rpc.NewClient(cfg.RPC.RPCURL)
	collector := rpc.NewCollector(
		client,
		cfg.RPC.PollInterval.Duration,
		cfg.RPC.DumpConsensusStateInterval.Duration,
		rpcOut,
		log,
	)
	go func() {
		if err := collector.Run(ctx); err != nil && ctx.Err() == nil {
			appLog.Error("rpc collector stopped", "err", err)
		}
	}()

	// Drain RPC payloads from channel into buffer.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case p := <-rpcOut:
				if dropped := buf.Push(p); dropped {
					appLog.Warn("rpc buffer full: oldest payload dropped")
				}
			}
		}
	}()

	// Per-minute stats ticker.
	statsTicker := time.NewTicker(statsInterval)
	defer statsTicker.Stop()

	// Flush buffer to watchtower on each poll interval.
	sendTicker := time.NewTicker(cfg.RPC.PollInterval.Duration)
	defer sendTicker.Stop()

	senderLog := log.With("component", "sender")

	for {
		select {
		case <-ctx.Done():
			return
		case <-statsTicker.C:
			logStats(appLog, st)
		case <-sendTicker.C:
			// Run flush in a goroutine so the ticker loop isn't blocked by retries.
			go flush(ctx, s, buf, senderLog, st)
		}
	}
}

func flush(ctx context.Context, s *sender.Sender, buf *sender.Buffer[protocol.RPCPayload], log *slog.Logger, st *stats.Stats) {
	items := buf.Drain()
	for _, p := range items {
		b, err := json.Marshal(p)
		if err != nil {
			log.Error("marshal payload", "err", err)
			continue
		}
		log.Debug("sending payload", "type", "rpc", "uncompressed_bytes", len(b))
		if err := s.SendWithRetry(ctx, "/rpc", p, maxSendAttempts, initialBackoff); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Error("send rpc payload", "err", err)
			continue
		}
		st.Record("rpc", len(b))
	}
}

func logStats(log *slog.Logger, st *stats.Stats) {
	snap, uptime := st.Snapshot()
	args := []any{"uptime", uptime.Round(time.Second)}
	for typ, s := range snap {
		args = append(args, slog.Group(typ,
			"last_min_bytes", s.LastMinuteBytes,
			"total_bytes", s.TotalBytes,
		))
	}
	log.Info("stats", args...)
}
