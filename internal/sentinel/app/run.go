// internal/sentinel/app/run.go
package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/aeddi/gno-watchtower/internal/sentinel/config"
	"github.com/aeddi/gno-watchtower/internal/sentinel/logs"
	"github.com/aeddi/gno-watchtower/internal/sentinel/metadata"
	"github.com/aeddi/gno-watchtower/internal/sentinel/otlp"
	"github.com/aeddi/gno-watchtower/internal/sentinel/resources"
	"github.com/aeddi/gno-watchtower/internal/sentinel/rpc"
	"github.com/aeddi/gno-watchtower/internal/sentinel/self"
	"github.com/aeddi/gno-watchtower/internal/sentinel/sender"
	"github.com/aeddi/gno-watchtower/internal/sentinel/stats"
	"github.com/aeddi/gno-watchtower/pkg/protocol"
)

const (
	// Per-collector bounded buffers. Sized so a few minutes of watchtower
	// downtime doesn't drop data. Overflow drops oldest (Buffer[T]) and is
	// recorded as sentinel_self_drops_total{reason="buffer_full"}.
	// Logs and rpc are the high-throughput paths; the rest are infrequent.
	rpcBufferSize       = 300
	logsBufferSize      = 500
	resourcesBufferSize = 20
	metadataBufferSize  = 10
	selfBufferSize      = 5
	otlpChannelSize     = 10
	maxSendAttempts     = 5
	initialBackoff      = time.Second
	statsInterval       = time.Minute
	logSendInterval     = time.Second
	metricsSendInterval = time.Second
)

// runCollector starts a collector goroutine tracked by wg. Transient errors
// are logged; ctx cancellation exits cleanly.
func runCollector(ctx context.Context, wg *sync.WaitGroup, name string, log *slog.Logger, run func(context.Context) error) {
	wg.Go(func() {
		if err := run(ctx); err != nil && ctx.Err() == nil {
			log.Error(name+" collector stopped", "err", err)
		}
	})
}

// wireBuffered launches a goroutine (tracked by wg) that drains outCh into buf
// until ctx is done.
func wireBuffered[T any](ctx context.Context, wg *sync.WaitGroup, outCh <-chan T, buf *sender.Buffer[T], log *slog.Logger, name string, st *stats.Stats) {
	wg.Go(func() {
		for {
			select {
			case <-ctx.Done():
				return
			case p := <-outCh:
				if dropped := buf.Push(p); dropped {
					log.Warn(name + " buffer full: oldest payload dropped")
					st.RecordDrop(name, "buffer_full")
				}
			}
		}
	})
}

// Run starts all enabled collectors and drains their output to the sender.
// It blocks until ctx is cancelled, then waits for every spawned goroutine
// (collectors, wireBuffered loops, health server, OTLP relay + sender) to
// finish before returning. This ordering guarantees no in-flight request
// outlives Run: main() can exit immediately after it returns.
func Run(ctx context.Context, cfg *config.Config, log *slog.Logger) {
	appLog := log.With("component", "app")
	senderLog := log.With("component", "sender")
	st := stats.New()
	noiseCfg, err := cfg.NoiseConfig()
	if err != nil {
		appLog.Error("load beacon keys", "err", err)
		return
	}
	s, err := sender.New(cfg.Server.URL, cfg.Server.Token, noiseCfg)
	if err != nil {
		appLog.Error("init sender", "err", err)
		return
	}

	var wg sync.WaitGroup
	defer wg.Wait()

	wg.Go(func() {
		runHealthServer(ctx, cfg.Health.Enabled, cfg.Health.ListenAddr, log.With("component", "health"))
	})

	if !cfg.RPC.Enabled && !cfg.Logs.Enabled && !cfg.OTLP.Enabled && !cfg.Resources.Enabled && !cfg.Metadata.Enabled && !cfg.Self.Enabled {
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
			cfg.RPC.GenesisRefreshInterval.Duration,
			cfg.RPC.ValidatorsRefreshInterval.Duration,
			rpcOut,
			log,
		)
		runCollector(ctx, &wg, "rpc", appLog, collector.Run)
		wireBuffered(ctx, &wg, rpcOut, rpcBuf, appLog, "rpc", st)

		t := time.NewTicker(cfg.RPC.PollInterval.Duration)
		defer t.Stop()
		rpcSendCh = t.C
	}

	// ---- Log collector
	var logBuf *sender.Buffer[protocol.LogPayload]
	var logSendCh <-chan time.Time
	if cfg.Logs.Enabled {
		src, err := logs.NewSource(cfg.Logs.Source, cfg.Logs.ContainerName, cfg.Logs.JournaldUnit, cfg.Logs.ResumeLookback.Duration)
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
			runCollector(ctx, &wg, "log", appLog, lc.Run)
			wireBuffered(ctx, &wg, logsOut, logBuf, appLog, "log", st)

			t := time.NewTicker(logSendInterval)
			defer t.Stop()
			logSendCh = t.C
		}
	}

	// ---- OTLP relay
	// OTLP bytes are forwarded immediately as received — no send ticker needed.
	if cfg.OTLP.Enabled {
		otlpOut := make(chan []byte, otlpChannelSize)
		onOTLPDrop := func() { st.RecordDrop("otlp", "buffer_full") }
		relay := otlp.NewRelay(cfg.OTLP.ListenAddr, otlpOut, onOTLPDrop, log)
		wg.Go(func() {
			if err := relay.Run(ctx); err != nil && ctx.Err() == nil {
				appLog.Error("otlp relay stopped", "err", err)
			}
		})
		wg.Go(func() {
			for {
				select {
				case <-ctx.Done():
					return
				case b := <-otlpOut:
					senderLog.Debug("sending payload", "type", "otlp", "bytes", len(b))
					if err := s.SendRawWithRetry(ctx, "/otlp", b, "application/x-protobuf", maxSendAttempts, initialBackoff); err != nil && ctx.Err() == nil {
						senderLog.Error("send otlp payload", "err", err)
						st.RecordDrop("otlp", "retry_exhausted")
						continue
					}
					st.Record("otlp", len(b), len(b))
				}
			}
		})
	}

	// ---- Resource collector
	var resourcesBuf *sender.Buffer[protocol.MetricsPayload]
	var resourcesSendCh <-chan time.Time
	if cfg.Resources.Enabled {
		resourcesBuf = sender.NewBuffer[protocol.MetricsPayload](resourcesBufferSize)
		resourcesOut := make(chan protocol.MetricsPayload, resourcesBufferSize)

		rc := resources.NewCollector(cfg.Resources, resourcesOut, log)
		runCollector(ctx, &wg, "resources", appLog, rc.Run)
		wireBuffered(ctx, &wg, resourcesOut, resourcesBuf, appLog, "resources", st)

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
		runCollector(ctx, &wg, "metadata", appLog, mc.Run)
		wireBuffered(ctx, &wg, metadataOut, metadataBuf, appLog, "metadata", st)

		t := time.NewTicker(metricsSendInterval)
		defer t.Stop()
		metadataSendCh = t.C
	}

	// ---- Self-stats collector
	var selfBuf *sender.Buffer[protocol.MetricsPayload]
	var selfSendCh <-chan time.Time
	if cfg.Self.Enabled {
		selfBuf = sender.NewBuffer[protocol.MetricsPayload](selfBufferSize)
		selfOut := make(chan protocol.MetricsPayload, selfBufferSize)

		sc := self.NewCollector(cfg.Self, st, selfOut, log)
		runCollector(ctx, &wg, "self", appLog, sc.Run)
		wireBuffered(ctx, &wg, selfOut, selfBuf, appLog, "self", st)

		t := time.NewTicker(metricsSendInterval)
		defer t.Stop()
		selfSendCh = t.C
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
		// Flushes run synchronously: if the watchtower is slow and a flush
		// runs past the next tick, the extra Ticker.Tick just gets coalesced
		// by the runtime (Tick drops to avoid piling up) and the collector's
		// buffer absorbs the latency. Spawning a goroutine per tick would
		// accumulate in-flight senders with no bound and race each other's
		// buf.Drain() calls.
		case <-rpcSendCh:
			flush(ctx, s, rpcBuf, senderLog, st)
			continue
		case <-logSendCh:
			flushLogs(ctx, s, logBuf, senderLog, st)
			continue
		case <-resourcesSendCh:
			flushMetrics(ctx, s, resourcesBuf, "resources", senderLog, st)
			continue
		case <-metadataSendCh:
			flushMetrics(ctx, s, metadataBuf, "metadata", senderLog, st)
			continue
		case <-selfSendCh:
			flushMetrics(ctx, s, selfBuf, "self", senderLog, st)
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
	if selfBuf != nil {
		flushMetrics(drainCtx, s, selfBuf, "self", senderLog, st)
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
			st.RecordDrop("rpc", "retry_exhausted")
			continue
		}
		st.Record("rpc", len(b), len(b))
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
			st.RecordDrop("logs", "retry_exhausted")
			continue
		}
		st.Record("logs", len(b), len(compressed))
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
			st.RecordDrop(typ, "retry_exhausted")
			continue
		}
		st.Record(typ, len(b), len(b))
	}
}

func logStats(log *slog.Logger, st *stats.Stats) {
	snap, uptime := st.Snapshot()
	args := []any{"uptime", uptime.Round(time.Second)}
	for typ, s := range snap {
		var lastDropTotal int64
		for _, n := range s.LastSnapshotDrops {
			lastDropTotal += n
		}
		args = append(args, slog.Group(typ,
			"last_snapshot_bytes", s.LastSnapshotBytes,
			"total_bytes", s.TotalBytes,
			"total_wire_bytes", s.TotalWireBytes,
			"last_snapshot_drops", lastDropTotal,
			"last_snapshot_drops_by_reason", s.LastSnapshotDrops,
		))
	}
	log.Info("stats", args...)
}
