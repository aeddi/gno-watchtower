// internal/sentinel/app/run.go
package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/gnolang/val-companion/internal/sentinel/config"
	"github.com/gnolang/val-companion/internal/sentinel/logs"
	"github.com/gnolang/val-companion/internal/sentinel/rpc"
	"github.com/gnolang/val-companion/internal/sentinel/sender"
	"github.com/gnolang/val-companion/internal/sentinel/stats"
	"github.com/gnolang/val-companion/pkg/protocol"
)

const (
	rpcBufferSize   = 100
	logsBufferSize  = 50
	maxSendAttempts = 5
	initialBackoff  = time.Second
	statsInterval   = time.Minute
	logSendInterval = time.Second
)

// Run starts all enabled collectors and drains their output to the sender.
// It blocks until ctx is cancelled.
func Run(ctx context.Context, cfg *config.Config, log *slog.Logger) {
	appLog := log.With("component", "app")
	senderLog := log.With("component", "sender")
	st := stats.New()
	s := sender.New(cfg.Server.URL, cfg.Server.Token)

	if !cfg.RPC.Enabled && !cfg.Logs.Enabled {
		<-ctx.Done()
		return
	}

	// ---- RPC collector
	var rpcBuf *sender.Buffer[protocol.RPCPayload]
	var rpcSendCh <-chan time.Time
	if cfg.RPC.Enabled {
		rpcBuf = sender.NewBuffer[protocol.RPCPayload](rpcBufferSize)
		rpcOut := make(chan protocol.RPCPayload, rpcBufferSize)

		client := rpc.NewClient(cfg.RPC.RPCURL)
		collector := rpc.NewCollector(
			client,
			cfg.RPC.PollInterval.Duration,
			cfg.RPC.DumpConsensusStateInterval.Duration,
			rpcOut,
			log,
		)
		go func() {
			// collect errors are transient; log and continue.
			if err := collector.Run(ctx); err != nil && ctx.Err() == nil {
				appLog.Error("rpc collector stopped", "err", err)
			}
		}()
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case p := <-rpcOut:
					if dropped := rpcBuf.Push(p); dropped {
						appLog.Warn("rpc buffer full: oldest payload dropped")
					}
				}
			}
		}()

		t := time.NewTicker(cfg.RPC.PollInterval.Duration)
		defer t.Stop()
		rpcSendCh = t.C
	}

	// ---- Log collector
	var logBuf *sender.Buffer[protocol.LogPayload]
	var logSendCh <-chan time.Time
	if cfg.Logs.Enabled {
		src, err := logs.NewSource(cfg.Logs.Source, cfg.Logs.ContainerName, cfg.Logs.JournaldUnit)
		if err != nil {
			appLog.Error("invalid log source config", "err", err)
		} else {
			logBuf = sender.NewBuffer[protocol.LogPayload](logsBufferSize)
			logsOut := make(chan protocol.LogPayload, logsBufferSize)

			lc := logs.NewCollector(
				src,
				cfg.Logs.MinLevel,
				int64(cfg.Logs.BatchSize),
				cfg.Logs.BatchTimeout.Duration,
				logsOut,
				log,
			)
			go func() {
				if err := lc.Run(ctx); err != nil && ctx.Err() == nil {
					appLog.Error("log collector stopped", "err", err)
				}
			}()
			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					case p := <-logsOut:
						if dropped := logBuf.Push(p); dropped {
							appLog.Warn("log buffer full: oldest payload dropped")
						}
					}
				}
			}()

			t := time.NewTicker(logSendInterval)
			defer t.Stop()
			logSendCh = t.C
		}
	}

	statsTicker := time.NewTicker(statsInterval)
	defer statsTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-statsTicker.C:
			logStats(appLog, st)
		case <-rpcSendCh:
			// Run flush in a goroutine so the ticker loop isn't blocked by retries.
			go flush(ctx, s, rpcBuf, senderLog, st)
		case <-logSendCh:
			// Run flush in a goroutine so the ticker loop isn't blocked by retries.
			go flushLogs(ctx, s, logBuf, senderLog, st)
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

func flushLogs(ctx context.Context, s *sender.Sender, buf *sender.Buffer[protocol.LogPayload], log *slog.Logger, st *stats.Stats) {
	items := buf.Drain()
	for _, p := range items {
		b, err := json.Marshal(p)
		if err != nil {
			log.Error("marshal log payload", "err", err)
			continue
		}
		log.Debug("sending payload", "type", "logs", "level", p.Level, "uncompressed_bytes", len(b))
		if err := s.SendCompressedWithRetry(ctx, "/logs", p, maxSendAttempts, initialBackoff); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Error("send log payload", "err", err)
			continue
		}
		st.Record("logs", len(b))
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
