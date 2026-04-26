package compactor

import (
	"context"
	"log/slog"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/store"
)

type Config struct {
	HotWindow  time.Duration
	WarmBucket time.Duration
	PruneAfter time.Duration
	CompactAt  string // "HH:MM" cluster local time, optional
	Jitter     time.Duration
}

type Compactor struct {
	store   store.Store
	cluster string
	cfg     Config
}

func New(s store.Store, cluster string, cfg Config) *Compactor {
	if cfg.HotWindow == 0 {
		cfg.HotWindow = 30 * 24 * time.Hour
	}
	if cfg.WarmBucket == 0 {
		cfg.WarmBucket = time.Minute
	}
	return &Compactor{store: s, cluster: cluster, cfg: cfg}
}

// RunOnce does a single compaction pass: hot → warm rollup, then prune.
func (c *Compactor) RunOnce(ctx context.Context) error {
	hotBoundary := time.Now().Add(-c.cfg.HotWindow)
	if rin, rout, err := c.store.CompactValidatorSamples(ctx, c.cluster, hotBoundary, c.cfg.WarmBucket); err != nil {
		return err
	} else {
		slog.InfoContext(ctx, "compacted samples_validator", "cluster", c.cluster, "rows_in", rin, "rows_out", rout)
	}
	if rin, rout, err := c.store.CompactChainSamples(ctx, c.cluster, hotBoundary, c.cfg.WarmBucket); err != nil {
		return err
	} else {
		slog.InfoContext(ctx, "compacted samples_chain", "cluster", c.cluster, "rows_in", rin, "rows_out", rout)
	}
	if c.cfg.PruneAfter > 0 {
		before := time.Now().Add(-c.cfg.PruneAfter)
		if err := c.store.PruneBefore(ctx, before); err != nil {
			return err
		}
	}
	return nil
}

// Run loops daily at CompactAt (or every 24h from start if CompactAt is empty).
func (c *Compactor) Run(ctx context.Context) error {
	for {
		next := nextRunTime(time.Now(), c.cfg.CompactAt, c.cfg.Jitter)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Until(next)):
		}
		if err := c.RunOnce(ctx); err != nil {
			slog.ErrorContext(ctx, "compactor run failed", "err", err)
		}
	}
}

func nextRunTime(now time.Time, hhmm string, jitter time.Duration) time.Time {
	if hhmm == "" {
		return now.Add(24 * time.Hour)
	}
	t, err := time.Parse("15:04", hhmm)
	if err != nil {
		return now.Add(24 * time.Hour)
	}
	target := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, now.Location())
	if !target.After(now) {
		target = target.Add(24 * time.Hour)
	}
	return target
}
