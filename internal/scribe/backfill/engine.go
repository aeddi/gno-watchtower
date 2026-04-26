package backfill

import (
	"context"
	"fmt"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/normalizer"
	"github.com/aeddi/gno-watchtower/internal/scribe/sources/loki"
	"github.com/aeddi/gno-watchtower/internal/scribe/sources/vm"
	"github.com/aeddi/gno-watchtower/internal/scribe/store"
)

// Deps holds all external collaborators the Engine needs.
type Deps struct {
	Store       store.Store
	ClusterID   string
	VM          *vm.Client
	Loki        *loki.Client
	FastQueries []string
	SlowQueries []string
	LogStreams  []string
	ChunkSize   time.Duration
	Normalizer  *normalizer.Normalizer
}

// Engine executes a BackfillJob in fixed-size chunks, persisting progress after each.
type Engine struct{ deps Deps }

// New constructs an Engine. ChunkSize defaults to one hour if zero.
func New(d Deps) *Engine {
	if d.ChunkSize <= 0 {
		d.ChunkSize = time.Hour
	}
	return &Engine{deps: d}
}

// Run walks the job's time range chunk by chunk, feeds results to the normalizer,
// and marks the job completed when every chunk succeeds.
func (e *Engine) Run(ctx context.Context, j store.BackfillJob) error {
	cursor := j.From
	if j.LastProcessedChunkEnd != nil && j.LastProcessedChunkEnd.After(cursor) {
		cursor = *j.LastProcessedChunkEnd
	}
	for cursor.Before(j.To) {
		end := cursor.Add(j.ChunkSize)
		if end.After(j.To) {
			end = j.To
		}
		if err := e.runChunk(ctx, cursor, end); err != nil {
			j.Status = "failed"
			j.ErrorCount++
			j.LastError = err.Error()
			j.LastProgressAt = time.Now()
			_ = e.deps.Store.UpsertBackfillJob(ctx, j)
			return err
		}
		j.LastProcessedChunkEnd = &end
		j.LastProgressAt = time.Now()
		if err := e.deps.Store.UpsertBackfillJob(ctx, j); err != nil {
			return err
		}
		cursor = end
	}
	j.Status = "completed"
	return e.deps.Store.UpsertBackfillJob(ctx, j)
}

func (e *Engine) runChunk(ctx context.Context, from, to time.Time) error {
	for _, q := range e.deps.FastQueries {
		series, err := e.deps.VM.Range(ctx, q, from, to, 3*time.Second)
		if err != nil {
			return fmt.Errorf("vm range %s: %w", q, err)
		}
		for _, s := range series {
			for i := range s.Values {
				e.deps.Normalizer.Input(normalizer.LaneFast) <- normalizer.Observation{
					Lane:         normalizer.LaneFast,
					IngestTime:   time.Now(),
					FromBackfill: true,
					Metric:       &s.Values[i],
					MetricQuery:  q,
				}
			}
		}
	}
	for _, q := range e.deps.SlowQueries {
		series, err := e.deps.VM.Range(ctx, q, from, to, time.Minute)
		if err != nil {
			return fmt.Errorf("vm slow range %s: %w", q, err)
		}
		for _, s := range series {
			for i := range s.Values {
				e.deps.Normalizer.Input(normalizer.LaneSlow) <- normalizer.Observation{
					Lane:         normalizer.LaneSlow,
					IngestTime:   time.Now(),
					FromBackfill: true,
					Metric:       &s.Values[i],
					MetricQuery:  q,
				}
			}
		}
	}
	for _, q := range e.deps.LogStreams {
		streams, err := e.deps.Loki.QueryRange(ctx, q, from, to, 5000)
		if err != nil {
			return fmt.Errorf("loki range %s: %w", q, err)
		}
		for _, s := range streams {
			for _, en := range s.Entries {
				ent := en
				e.deps.Normalizer.Input(normalizer.LaneLogs) <- normalizer.Observation{
					Lane:         normalizer.LaneLogs,
					IngestTime:   time.Now(),
					FromBackfill: true,
					LogQuery:     q,
					LogEntry:     &loki.TailEntry{Stream: s, Time: ent.Time, Line: ent.Line},
				}
			}
		}
	}
	return nil
}
