package logs

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	pkglogger "github.com/aeddi/gno-watchtower/pkg/logger"
	"github.com/aeddi/gno-watchtower/pkg/protocol"
)

// lineBufferSize is the capacity of the internal channel between the Source goroutine
// and the collection loop.
const lineBufferSize = 256

// Collector reads from a Source, filters by min_level, accumulates log lines by level,
// and flushes protocol.LogPayload values when either batchSize bytes accumulate or
// batchTimeout elapses. One payload per level per flush.
type Collector struct {
	source       Source
	minLevel     int           // numeric rank; lines with rank < minLevel are dropped
	batchSize    int64         // flush when accumulated bytes reach this threshold
	batchTimeout time.Duration
	out          chan<- protocol.LogPayload
	log          *slog.Logger
}

// NewCollector creates a Collector.
// minLevel must be one of "debug","info","warn","error" (unknown → treated as "info").
// batchSize is in bytes.
func NewCollector(source Source, minLevel string, batchSize int64, batchTimeout time.Duration, out chan<- protocol.LogPayload, log *slog.Logger) *Collector {
	return &Collector{
		source:       source,
		minLevel:     pkglogger.LevelRank(minLevel),
		batchSize:    batchSize,
		batchTimeout: batchTimeout,
		out:          out,
		log:          log.With("component", "log_collector"),
	}
}

// ReconnectBackoff is how long the Tail goroutine sleeps before reconnecting
// after the source returns (e.g. because the source container restarted).
// Exposed as a var (not a const) so tests can set it shorter.
var ReconnectBackoff = 2 * time.Second

// Run starts the collection loop. It blocks until ctx is cancelled.
func (c *Collector) Run(ctx context.Context) error {
	lineCh := make(chan LogLine, lineBufferSize)
	go func() {
		// Reconnect loop: if the source's Tail returns (source container
		// restarted, network hiccup, …), sleep briefly and call it again.
		// Without this loop the collector goes silent after the first
		// disconnect until the entire sentinel process restarts.
		for ctx.Err() == nil {
			if err := c.source.Tail(ctx, lineCh); err != nil && ctx.Err() == nil {
				c.log.Error("source error, reconnecting", "err", err, "backoff", ReconnectBackoff)
			}
			select {
			case <-time.After(ReconnectBackoff):
			case <-ctx.Done():
				return
			}
		}
	}()

	// accumulated[level] holds raw JSON lines waiting to be flushed.
	accumulated := make(map[string][]json.RawMessage)
	var totalBytes int64

	flush := func() {
		for level, lines := range accumulated {
			c.log.Debug("flushing batch", "level", level, "lines", len(lines))
			// Flush is best-effort at shutdown: if out is full and ctx is cancelled,
			// remaining levels are dropped.
			select {
			case c.out <- protocol.LogPayload{Level: level, Lines: lines}:
			case <-ctx.Done():
				return
			}
		}
		accumulated = make(map[string][]json.RawMessage)
		totalBytes = 0
	}

	ticker := time.NewTicker(c.batchTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			flush()
			return ctx.Err()
		case line := <-lineCh:
			if pkglogger.LevelRank(line.Level) < c.minLevel {
				continue
			}
			accumulated[line.Level] = append(accumulated[line.Level], line.Raw)
			totalBytes += int64(len(line.Raw))
			if totalBytes >= c.batchSize {
				flush()
				ticker.Reset(c.batchTimeout)
			}
		case <-ticker.C:
			flush()
		}
	}
}
