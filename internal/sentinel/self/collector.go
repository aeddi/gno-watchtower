// Package self implements a sentinel-side self-stats collector. It reads the
// shared stats.Stats accumulator on a ticker and emits a MetricsPayload with
// a "self_stats" key that the watchtower's metrics forwarder extracts into
// Prometheus counters (sentinel_self_bytes_sent_total, sentinel_self_drops_total,
// sentinel_self_retries_total). Gives operators per-validator pipeline health
// that would otherwise be invisible beyond the sentinel's own stdout log.
package self

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/aeddi/gno-watchtower/internal/sentinel/config"
	"github.com/aeddi/gno-watchtower/internal/sentinel/stats"
	"github.com/aeddi/gno-watchtower/pkg/protocol"
)

// Collector periodically emits the process-wide stats snapshot as a metrics
// payload. It carries no collector-specific configuration beyond its ticker.
type Collector struct {
	cfg   config.SelfConfig
	stats *stats.Stats
	out   chan<- protocol.MetricsPayload
	log   *slog.Logger
}

// NewCollector creates a self-stats Collector.
func NewCollector(cfg config.SelfConfig, st *stats.Stats, out chan<- protocol.MetricsPayload, log *slog.Logger) *Collector {
	return &Collector{
		cfg:   cfg,
		stats: st,
		out:   out,
		log:   log.With("component", "self_collector"),
	}
}

// TypeStats is the wire shape of per-type counters carried inside the
// "self_stats" entry of a MetricsPayload.
// All fields are absolute counters — the backend applies rate()/increase()
// to derive deltas.
type TypeStats struct {
	UncompressedBytes int64 `json:"uncompressed_bytes"`
	WireBytes         int64 `json:"wire_bytes"`
	Drops             int64 `json:"drops"`
	Retries           int64 `json:"retries"`
}

// Run ticks on cfg.ReportInterval, serialises the current stats snapshot, and
// pushes it to out until ctx is cancelled.
func (c *Collector) Run(ctx context.Context) error {
	ticker := time.NewTicker(c.cfg.ReportInterval.Duration)
	defer ticker.Stop()

	// Emit once on startup so the series appear immediately in VM rather than
	// waiting a full interval after cold-start.
	c.emit(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			c.emit(ctx)
		}
	}
}

func (c *Collector) emit(ctx context.Context) {
	snap, _ := c.stats.Snapshot()
	if len(snap) == 0 {
		return
	}
	byType := make(map[string]TypeStats, len(snap))
	for typ, s := range snap {
		byType[typ] = TypeStats{
			UncompressedBytes: s.TotalBytes,
			WireBytes:         s.TotalWireBytes,
			Drops:             s.TotalDrops,
			Retries:           s.TotalRetries,
		}
	}
	raw, err := json.Marshal(byType)
	if err != nil {
		c.log.Warn("marshal self stats", "err", err)
		return
	}
	payload := protocol.MetricsPayload{
		CollectedAt: time.Now().UTC(),
		Data: map[string]json.RawMessage{
			"self_stats": raw,
		},
	}
	select {
	case c.out <- payload:
	case <-ctx.Done():
	}
}
