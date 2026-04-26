package backfill

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/store"
)

type SchedulerConfig struct {
	PollInterval     time.Duration
	ResumeStaleAfter time.Duration
}

type Scheduler struct {
	store   store.Store
	engine  *Engine
	cluster string
	cfg     SchedulerConfig
	mu      sync.Mutex
	running map[string]struct{}
}

func NewScheduler(s store.Store, e *Engine, cluster string, cfg SchedulerConfig) *Scheduler {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 5 * time.Second
	}
	if cfg.ResumeStaleAfter <= 0 {
		cfg.ResumeStaleAfter = 5 * time.Minute
	}
	return &Scheduler{
		store:   s,
		engine:  e,
		cluster: cluster,
		cfg:     cfg,
		running: map[string]struct{}{},
	}
}

func (sc *Scheduler) Run(ctx context.Context) error {
	t := time.NewTicker(sc.cfg.PollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			sc.tick(ctx)
		}
	}
}

func (sc *Scheduler) tick(ctx context.Context) {
	jobs, err := sc.store.ListBackfillJobs(ctx, sc.cluster, 50)
	if err != nil {
		slog.WarnContext(ctx, "backfill list failed", "err", err)
		return
	}
	for _, j := range jobs {
		runnable := j.Status == "pending" ||
			(j.Status == "running" && time.Since(j.LastProgressAt) > sc.cfg.ResumeStaleAfter)
		if !runnable {
			continue
		}
		sc.mu.Lock()
		if _, busy := sc.running[j.ID]; busy {
			sc.mu.Unlock()
			continue
		}
		sc.running[j.ID] = struct{}{}
		sc.mu.Unlock()

		// Mark as running before spawning so concurrent ticks see the state
		// immediately even if the job's actual processing is slow.
		j.Status = "running"
		j.LastProgressAt = time.Now()
		if err := sc.store.UpsertBackfillJob(ctx, j); err != nil {
			sc.releaseJob(j.ID)
			slog.ErrorContext(ctx, "backfill mark-running failed", "id", j.ID, "err", err)
			continue
		}

		go func(j store.BackfillJob) {
			defer sc.releaseJob(j.ID)
			if err := sc.engine.Run(ctx, j); err != nil {
				slog.ErrorContext(ctx, "backfill engine failed", "id", j.ID, "err", err)
			}
		}(j)
	}
}

func (sc *Scheduler) releaseJob(id string) {
	sc.mu.Lock()
	delete(sc.running, id)
	sc.mu.Unlock()
}
