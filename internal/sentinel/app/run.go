// internal/sentinel/app/run.go
package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/aeddi/gno-watchtower/internal/sentinel/config"
	"github.com/aeddi/gno-watchtower/internal/sentinel/logs"
	"github.com/aeddi/gno-watchtower/internal/sentinel/metadata"
	"github.com/aeddi/gno-watchtower/internal/sentinel/otlp"
	"github.com/aeddi/gno-watchtower/internal/sentinel/resources"
	"github.com/aeddi/gno-watchtower/internal/sentinel/rpc"
	"github.com/aeddi/gno-watchtower/internal/sentinel/sender"
	"github.com/aeddi/gno-watchtower/internal/sentinel/stats"
	"github.com/aeddi/gno-watchtower/pkg/protocol"
)

const (
	rpcBufferSize       = 100
	logsBufferSize      = 50
	resourcesBufferSize = 20
	metadataBufferSize  = 10
	otlpChannelSize     = 10
	maxSendAttempts     = 5
	initialBackoff      = time.Second
	statsInterval       = time.Minute
	logSendInterval     = time.Second
	metricsSendInterval = time.Second
)

// runCollector starts a collector goroutine. Transient errors are logged; ctx cancellation exits cleanly.
func runCollector(ctx context.Context, name string, log *slog.Logger, run func(context.Context) error) {
	go func() {
		if err := run(ctx); err != nil && ctx.Err() == nil {
			log.Error(name+" collector stopped", "err", err)
		}
	}()
}

// wireBuffered launches a goroutine that drains outCh into buf until ctx is done.
func wireBuffered[T any](ctx context.Context, outCh <-chan T, buf *sender.Buffer[T], log *slog.Logger, name string, st *stats.Stats) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case p := <-outCh:
				if dropped := buf.Push(p); dropped {
					log.Warn(name + " buffer full: oldest payload dropped")
					st.RecordDrop(name)
				}
			}
		}
	}()
}

// Run starts all enabled collectors and drains their output to the sender.
// It blocks until ctx is cancelled.
func Run(ctx context.Context, cfg *config.Config, log *slog.Logger) {
	appLog := log.With("component", "app")
	go runHealthServer(ctx, cfg.Health.Enabled, cfg.Health.ListenAddr, log.With("component", "health"))
	senderLog := log.With("component", "sender")
	st := stats.New()
	s := sender.New(cfg.Server.URL, cfg.Server.Token)

	if !cfg.RPC.Enabled && !cfg.Logs.Enabled && !cfg.OTLP.Enabled && !cfg.Resources.Enabled && !cfg.Metadata.Enabled {
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
		runCollector(ctx, "rpc", appLog, collector.Run)
		wireBuffered(ctx, rpcOut, rpcBuf, appLog, "rpc", st)

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
			runCollector(ctx, "log", appLog, lc.Run)
			wireBuffered(ctx, logsOut, logBuf, appLog, "log", st)

			t := time.NewTicker(logSendInterval)
			defer t.Stop()
			logSendCh = t.C
		}
	}

	// ---- OTLP relay
	// OTLP bytes are forwarded immediately as received — no send ticker needed.
	if cfg.OTLP.Enabled {
		otlpOut := make(chan []byte, otlpChannelSize)
		relay := otlp.NewRelay(cfg.OTLP.ListenAddr, otlpOut, log)
		go func() {
			if err := relay.Run(ctx); err != nil && ctx.Err() == nil {
				appLog.Error("otlp relay stopped", "err", err)
			}
		}()
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case b := <-otlpOut:
					senderLog.Debug("sending payload", "type", "otlp", "bytes", len(b))
					if err := s.SendRawWithRetry(ctx, "/otlp", b, "application/x-protobuf", maxSendAttempts, initialBackoff); err != nil && ctx.Err() == nil {
						senderLog.Error("send otlp payload", "err", err)
						continue
					}
					st.Record("otlp", len(b))
				}
			}
		}()
	}

	// ---- Resource collector
	var resourcesBuf *sender.Buffer[protocol.MetricsPayload]
	var resourcesSendCh <-chan time.Time
	if cfg.Resources.Enabled {
		resourcesBuf = sender.NewBuffer[protocol.MetricsPayload](resourcesBufferSize)
		resourcesOut := make(chan protocol.MetricsPayload, resourcesBufferSize)

		rc := resources.NewCollector(cfg.Resources, resourcesOut, log)
		runCollector(ctx, "resources", appLog, rc.Run)
		wireBuffered(ctx, resourcesOut, resourcesBuf, appLog, "resources", st)

		t := time.NewTicker(metricsSendInterval)
		defer t.Stop()
		resourcesSendCh = t.C
	}

	// ---- Metadata collector
	var metadataBuf *sender.Buffer[protocol.MetricsPayload]
	var metadataSendCh <-chan time.Time
	if cfg.Metadata.Enabled {
		metadataBuf = sender.NewBuffer[protocol.MetricsPayload](metadataBufferSize)
		metadataOut := make(chan protocol.MetricsPayload, metadataBufferSize)

		mc := metadata.NewCollector(cfg.Metadata, metadataOut, log)
		runCollector(ctx, "metadata", appLog, mc.Run)
		wireBuffered(ctx, metadataOut, metadataBuf, appLog, "metadata", st)

		t := time.NewTicker(metricsSendInterval)
		defer t.Stop()
		metadataSendCh = t.C
	}

	statsTicker := time.NewTicker(statsInterval)
	defer statsTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			// fall through to drain
		case <-statsTicker.C:
			logStats(appLog, st)
			continue
		case <-rpcSendCh:
			// Run flush in a goroutine so the ticker loop isn't blocked by retries.
			go flush(ctx, s, rpcBuf, senderLog, st)
			continue
		case <-logSendCh:
			// Run flush in a goroutine so the ticker loop isn't blocked by retries.
			go flushLogs(ctx, s, logBuf, senderLog, st)
			continue
		case <-resourcesSendCh:
			// Run flush in a goroutine so the ticker loop isn't blocked by retries.
			go flushMetrics(ctx, s, resourcesBuf, "resources", senderLog, st)
			continue
		case <-metadataSendCh:
			// Run flush in a goroutine so the ticker loop isn't blocked by retries.
			go flushMetrics(ctx, s, metadataBuf, "metadata", senderLog, st)
			continue
		}
		break
	}

	// Gracefully flush remaining buffered payloads.
	// Use a fresh context so flushing isn't blocked by the cancelled ctx.
	drainCtx, drainCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer drainCancel()
	if rpcBuf != nil {
		flush(drainCtx, s, rpcBuf, senderLog, st)
	}
	if logBuf != nil {
		flushLogs(drainCtx, s, logBuf, senderLog, st)
	}
	if resourcesBuf != nil {
		flushMetrics(drainCtx, s, resourcesBuf, "resources", senderLog, st)
	}
	if metadataBuf != nil {
		flushMetrics(drainCtx, s, metadataBuf, "metadata", senderLog, st)
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
		log.Debug("sending payload", "type", "rpc", "bytes", len(b))
		if err := s.SendRawWithRetry(ctx, "/rpc", b, "application/json", maxSendAttempts, initialBackoff); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Error("send rpc payload", "err", err)
			st.RecordRetry("rpc")
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
		compressed := sender.ZstdCompress(b)
		log.Debug("sending payload", "type", "logs", "level", p.Level, "uncompressed_bytes", len(b), "wire_bytes", len(compressed))
		if err := s.SendCompressedBytesWithRetry(ctx, "/logs", compressed, maxSendAttempts, initialBackoff); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Error("send log payload", "err", err)
			st.RecordRetry("logs")
			continue
		}
		st.Record("logs", len(b))
	}
}

func flushMetrics(ctx context.Context, s *sender.Sender, buf *sender.Buffer[protocol.MetricsPayload], typ string, log *slog.Logger, st *stats.Stats) {
	items := buf.Drain()
	for _, p := range items {
		b, err := json.Marshal(p)
		if err != nil {
			log.Error("marshal metrics payload", "err", err)
			continue
		}
		log.Debug("sending payload", "type", typ, "bytes", len(b))
		if err := s.SendRawWithRetry(ctx, "/metrics", b, "application/json", maxSendAttempts, initialBackoff); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Error("send metrics payload", "err", err, "type", typ)
			st.RecordRetry(typ)
			continue
		}
		st.Record(typ, len(b))
	}
}

func logStats(log *slog.Logger, st *stats.Stats) {
	snap, uptime := st.Snapshot()
	args := []any{"uptime", uptime.Round(time.Second)}
	for typ, s := range snap {
		args = append(args, slog.Group(typ,
			"last_min_bytes", s.LastMinuteBytes,
			"total_bytes", s.TotalBytes,
			"drops", s.Drops,
			"retries", s.Retries,
		))
	}
	log.Info("stats", args...)
}
