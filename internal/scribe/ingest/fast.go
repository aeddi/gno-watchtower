package ingest

import (
	"context"
	"log/slog"
	"math"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/normalizer"
	"github.com/aeddi/gno-watchtower/internal/scribe/sources/vm"
)

type FastLane struct {
	client   *vm.Client
	queries  []string
	interval time.Duration
	out      chan<- normalizer.Observation
}

func NewFastLane(c *vm.Client, queries []string, interval time.Duration, out chan<- normalizer.Observation) *FastLane {
	if interval <= 0 {
		interval = 3 * time.Second
	}
	return &FastLane{client: c, queries: queries, interval: interval, out: out}
}

func (l *FastLane) Run(ctx context.Context) error {
	t := time.NewTicker(l.interval)
	defer t.Stop()
	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			now := time.Now().UTC()
			anyErr := false
			for _, q := range l.queries {
				samples, err := l.client.Instant(ctx, q, now)
				if err != nil {
					slog.WarnContext(ctx, "vm instant failed", "lane", "fast", "query", q, "err", err)
					anyErr = true
					continue
				}
				for i := range samples {
					select {
					case l.out <- normalizer.Observation{
						Lane: normalizer.LaneFast, IngestTime: now,
						Metric: &samples[i], MetricQuery: q,
					}:
					case <-ctx.Done():
						return ctx.Err()
					default:
						// Buffer full — caller's responsibility to surface metric.
					}
				}
			}
			if anyErr {
				time.Sleep(backoff)
				backoff = time.Duration(math.Min(float64(backoff*2), float64(60*time.Second)))
			} else {
				backoff = time.Second
			}
		}
	}
}
