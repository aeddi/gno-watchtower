package ingest

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/normalizer"
	"github.com/aeddi/gno-watchtower/internal/scribe/sources/loki"
)

// LogsLane fans out one Loki tail websocket per LogQL stream, forwarding each
// line as a normalizer.Observation on the output channel. It reconnects on
// disconnect, replaying the configured overlap window to avoid gaps.
type LogsLane struct {
	base          string
	streams       []string
	overlapWindow time.Duration
	out           chan<- normalizer.Observation
}

// NewLogsLane returns a LogsLane ready to run.
func NewLogsLane(base string, streams []string, overlap time.Duration, out chan<- normalizer.Observation) *LogsLane {
	return &LogsLane{base: base, streams: streams, overlapWindow: overlap, out: out}
}

// Run starts one goroutine per stream and waits for all to exit. It returns
// ctx.Err() once the context is done.
func (l *LogsLane) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	for _, q := range l.streams {
		q := q
		wg.Go(func() { l.tailLoop(ctx, q) })
	}
	wg.Wait()
	return ctx.Err()
}

// tailLoop dials Loki's tail endpoint for the given query and forwards entries
// to the output channel. It reconnects with exponential backoff on disconnect.
func (l *LogsLane) tailLoop(ctx context.Context, q string) {
	backoff := time.Second
	lastSeen := time.Now().UTC()
	for {
		if ctx.Err() != nil {
			return
		}
		ch := make(chan loki.TailEntry, 64)
		errCh := make(chan error, 1)
		go func() {
			errCh <- loki.Tail(ctx, l.base, q, lastSeen.Add(-l.overlapWindow), ch)
		}()
	read:
		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-ch:
				if !ok {
					break read
				}
				lastSeen = e.Time
				select {
				case l.out <- normalizer.Observation{
					Lane: normalizer.LaneLogs, IngestTime: time.Now().UTC(),
					LogEntry: &e, LogQuery: q,
				}:
				default:
					// Buffer full — drop and let self-stats surface the loss.
				}
			case err := <-errCh:
				if err != nil && ctx.Err() == nil {
					slog.WarnContext(ctx, "loki tail disconnected", "query", q, "err", err)
				}
				break read
			}
		}
		t := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
		if backoff < 60*time.Second {
			backoff *= 2
		}
	}
}
