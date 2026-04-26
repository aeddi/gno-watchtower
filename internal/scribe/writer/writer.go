// Package writer provides the single-writer goroutine that fronts the Store.
// All ingestion lanes feed into Submit; this is the only path that calls
// Store.WriteBatch. After each successful commit it fans out the committed
// events to SSE subscribers.
package writer

import (
	"context"
	"sync"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/store"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

// Config holds tunables for the Writer.
type Config struct {
	// BatchSize is the maximum number of items (across all op kinds) to
	// accumulate before an early flush. Zero defaults to 100.
	BatchSize int
	// BatchWindow is the maximum time to wait before flushing a partial batch.
	// Zero defaults to 250ms.
	BatchWindow time.Duration
}

// Writer is the single-writer goroutine fronting the Store. It accepts Ops on
// two priority channels (live and backfill), batches them, persists via
// WriteBatch, and broadcasts committed events to SSE subscribers.
type Writer struct {
	store store.Store
	cfg   Config

	live     chan types.Op
	backfill chan types.Op

	mu          sync.Mutex
	subscribers map[chan types.Event]struct{}
}

// New returns a Writer backed by s. Run must be called to start processing.
func New(s store.Store, cfg Config) *Writer {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.BatchWindow <= 0 {
		cfg.BatchWindow = 250 * time.Millisecond
	}
	return &Writer{
		store:       s,
		cfg:         cfg,
		live:        make(chan types.Op, 4096),
		backfill:    make(chan types.Op, 4096),
		subscribers: map[chan types.Event]struct{}{},
	}
}

// Submit enqueues op for processing. Live ops drain before backfill ops.
// If the target channel is full the op is dropped rather than blocking the
// caller; the producing lane is responsible for incrementing any drop counter.
func (w *Writer) Submit(op types.Op) {
	ch := w.live
	if op.FromBackfill {
		ch = w.backfill
	}
	select {
	case ch <- op:
	default:
	}
}

// Subscribe returns a buffered channel that receives every event committed by
// the Writer. buffer controls the slow-subscriber drop threshold. The channel
// is closed by Unsubscribe.
func (w *Writer) Subscribe(buffer int) chan types.Event {
	ch := make(chan types.Event, buffer)
	w.mu.Lock()
	w.subscribers[ch] = struct{}{}
	w.mu.Unlock()
	return ch
}

// Unsubscribe removes ch from the fan-out set and closes it.
func (w *Writer) Unsubscribe(ch chan types.Event) {
	w.mu.Lock()
	if _, ok := w.subscribers[ch]; ok {
		delete(w.subscribers, ch)
		close(ch)
	}
	w.mu.Unlock()
}

// Run is the single writer goroutine. It exits when ctx is cancelled, flushing
// any pending batch first.
func (w *Writer) Run(ctx context.Context) error {
	tick := time.NewTimer(w.cfg.BatchWindow)
	defer tick.Stop()

	batch := store.Batch{}
	committed := []types.Event{}

	flush := func() {
		total := len(batch.Events) + len(batch.SamplesValidator) +
			len(batch.SamplesChain) + len(batch.Anchors)
		if total == 0 {
			return
		}
		// Best-effort retry once on transient error.
		for attempt := 0; attempt < 2; attempt++ {
			err := w.store.WriteBatch(ctx, batch)
			if err == nil {
				break
			}
			if attempt == 1 {
				panic(err)
			}
			time.Sleep(50 * time.Millisecond)
		}
		w.broadcast(committed)
		batch = store.Batch{}
		committed = nil
	}

	add := func(op types.Op) {
		switch op.Kind {
		case types.OpInsertEvent:
			batch.Events = append(batch.Events, *op.Event)
			committed = append(committed, *op.Event)
		case types.OpUpsertSampleValidator:
			batch.SamplesValidator = append(batch.SamplesValidator, *op.SampleValid)
		case types.OpUpsertSampleChain:
			batch.SamplesChain = append(batch.SamplesChain, *op.SampleChain)
		case types.OpInsertAnchor:
			batch.Anchors = append(batch.Anchors, *op.Anchor)
		}
	}

	totalQueued := func() int {
		return len(batch.Events) + len(batch.SamplesValidator) +
			len(batch.SamplesChain) + len(batch.Anchors)
	}

	for {
		// Strict priority: drain live before touching backfill.
		select {
		case <-ctx.Done():
			flush()
			return ctx.Err()
		case op := <-w.live:
			add(op)
		default:
			select {
			case <-ctx.Done():
				flush()
				return ctx.Err()
			case op := <-w.live:
				add(op)
			case op := <-w.backfill:
				add(op)
			case <-tick.C:
				flush()
				tick.Reset(w.cfg.BatchWindow)
				continue
			}
		}

		if totalQueued() >= w.cfg.BatchSize {
			flush()
			if !tick.Stop() {
				select {
				case <-tick.C:
				default:
				}
			}
			tick.Reset(w.cfg.BatchWindow)
		}
	}
}

func (w *Writer) broadcast(events []types.Event) {
	if len(events) == 0 {
		return
	}
	w.mu.Lock()
	subs := make([]chan types.Event, 0, len(w.subscribers))
	for ch := range w.subscribers {
		subs = append(subs, ch)
	}
	w.mu.Unlock()

	for _, ch := range subs {
		for _, e := range events {
			select {
			case ch <- e:
			default:
				// Slow subscriber — drop event.
			}
		}
	}
}
