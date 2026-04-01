package logs

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/gnolang/val-companion/pkg/protocol"
)

// levelRank returns a numeric rank for min_level filtering.
// Unknown levels → rank 1 (info).
func levelRank(level string) int {
	switch level {
	case "debug":
		return 0
	case "info":
		return 1
	case "warn":
		return 2
	case "error":
		return 3
	default:
		return 1
	}
}

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
		minLevel:     levelRank(minLevel),
		batchSize:    batchSize,
		batchTimeout: batchTimeout,
		out:          out,
		log:          log.With("component", "log_collector"),
	}
}

// Run starts the collection loop. It blocks until ctx is cancelled.
func (c *Collector) Run(ctx context.Context) error {
	lineCh := make(chan LogLine, 256)
	go func() {
		// collect errors are transient; log and continue.
		if err := c.source.Tail(ctx, lineCh); err != nil && ctx.Err() == nil {
			c.log.Error("source error", "err", err)
		}
	}()

	// accumulated[level] holds raw JSON lines waiting to be flushed.
	accumulated := make(map[string][]json.RawMessage)
	var totalBytes int64

	flush := func() {
		for level, lines := range accumulated {
			c.log.Debug("flushing batch", "level", level, "lines", len(lines))
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
			if levelRank(line.Level) < c.minLevel {
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
