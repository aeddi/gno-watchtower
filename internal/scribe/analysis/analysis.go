package analysis

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/scribemetrics"
	"github.com/aeddi/gno-watchtower/internal/scribe/store"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

// Rehydrate populates each rule's Tracker from currently-open diagnostic
// rows in the store. Must be called before Start. Idempotent — calling it
// twice replaces tracker state with the latest DB read.
//
// Rules opt into recovery tracking by exposing RecoveryTracker() *Tracker.
// Point-in-time rules (no recovery) implement no such method and are skipped.
func (e *Engine) Rehydrate(ctx context.Context, st store.Store) error {
	for _, w := range e.workers {
		tr, ok := w.rule.(interface{ RecoveryTracker() *Tracker })
		if !ok {
			continue
		}
		entries, err := queryOpenIncidents(ctx, st, e.deps.ClusterID, w.meta.Kind())
		if err != nil {
			return fmt.Errorf("rehydrate %s: %w", w.meta.Kind(), err)
		}
		tr.RecoveryTracker().Rehydrate(entries)
		if e.metrics != nil && e.metrics.AnalysisOpenIncidents != nil {
			e.metrics.AnalysisOpenIncidents.WithLabelValues(w.meta.Kind()).Set(float64(len(entries)))
		}
	}
	return nil
}

// queryOpenIncidents returns key->event_id for every diagnostic row of `kind`
// that is `state='open'` and not yet recovered. The recovery key lives in
// payload.recovery_key — rules that use Tracker MUST populate it on the
// opening emission so rehydration can reconstruct tracker state.
func queryOpenIncidents(ctx context.Context, st store.Store, cluster, kind string) (map[string]string, error) {
	opens, _, err := st.QueryEvents(ctx, store.EventQuery{
		ClusterID: cluster, Kind: kind, State: "open", Limit: 1000,
	})
	if err != nil {
		return nil, err
	}
	recovers, _, err := st.QueryEvents(ctx, store.EventQuery{
		ClusterID: cluster, Kind: kind, State: "recovered", Limit: 1000,
	})
	if err != nil {
		return nil, err
	}
	recovered := map[string]bool{}
	for _, r := range recovers {
		if r.Recovers != "" {
			recovered[r.Recovers] = true
		}
	}
	out := map[string]string{}
	for _, o := range opens {
		if recovered[o.EventID] {
			continue
		}
		key, _ := o.Payload["recovery_key"].(string)
		if key == "" {
			continue
		}
		out[key] = o.EventID
	}
	return out, nil
}

// Broadcaster is the narrow surface of *writer.Writer the engine consumes.
// Defined as an interface so tests can substitute a stub.
type Broadcaster interface {
	Subscribe(buffer int) chan types.Event
	Unsubscribe(ch chan types.Event)
}

// EngineConfig holds engine-wide tunables. Per-rule config (enable/threshold)
// lives in the rule's RuleConfig (see config.go).
type EngineConfig struct {
	// QueueSize is the per-rule trigger queue capacity. Default 1024.
	QueueSize int
	// SubscribeBuffer is the buffer size of the Subscribe channel. Default 1024.
	SubscribeBuffer int
	// Disabled lists rule kinds (e.g. "diagnostic.block_missed_v1") that should
	// not be registered with workers. Driven by [analysis] config.
	Disabled []string
	// RuleOverlays maps rule kind -> raw map[string]any from
	// [analysis.rules.<kind>] for RuleConfig construction.
	RuleOverlays map[string]map[string]any
}

// Engine subscribes to the writer broadcaster and dispatches events to per-rule
// workers. Construct via New, then Start.
type Engine struct {
	deps    Deps
	bc      Broadcaster
	w       writerSubmitter
	metrics *scribemetrics.Registry
	cfg     EngineConfig

	workers []*ruleWorker
}

// New constructs an Engine with all currently-registered rules. Returns an
// error if any rule's RuleConfig fails to validate.
func New(deps Deps, bc Broadcaster, w writerSubmitter, m *scribemetrics.Registry, cfg EngineConfig) (*Engine, error) {
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 1024
	}
	if cfg.SubscribeBuffer <= 0 {
		cfg.SubscribeBuffer = 1024
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}

	disabled := map[string]bool{}
	for _, k := range cfg.Disabled {
		disabled[k] = true
	}

	var workers []*ruleWorker
	for _, kind := range RegisteredCodes() {
		if disabled[kind] {
			continue
		}
		r := Lookup(kind)
		meta := GetMeta(kind)
		rc, err := NewRuleConfig(meta, cfg.RuleOverlays[kind])
		if err != nil {
			return nil, fmt.Errorf("rule %s: %w", kind, err)
		}
		workers = append(workers, newRuleWorker(r, deps, rc, w, m, cfg.QueueSize))
	}

	return &Engine{
		deps:    deps,
		bc:      bc,
		w:       w,
		metrics: m,
		cfg:     cfg,
		workers: workers,
	}, nil
}

// Start launches the dispatcher and per-rule worker goroutines. Returns
// immediately. Engine shuts down when ctx is cancelled.
func (e *Engine) Start(ctx context.Context) error {
	for _, w := range e.workers {
		go w.run(ctx)
		if w.meta.TickPeriod > 0 {
			go w.tickLoop(ctx)
		}
	}
	go e.dispatchLoop(ctx)
	return nil
}

func (e *Engine) dispatchLoop(ctx context.Context) {
	sub := e.bc.Subscribe(e.cfg.SubscribeBuffer)
	defer e.bc.Unsubscribe(sub)

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-sub:
			if !ok {
				return
			}
			trigger := ev // copy so &trigger is stable across the worker loop
			for _, w := range e.workers {
				if !KindMatch(w.meta.Kinds, trigger.Kind) {
					continue
				}
				select {
				case w.queue <- Trigger{Event: &trigger}:
				default:
					if e.metrics != nil && e.metrics.AnalysisQueueDrops != nil {
						e.metrics.AnalysisQueueDrops.WithLabelValues(w.meta.Kind()).Inc()
					}
				}
			}
		}
	}
}

// ruleWorker is one rule's bounded queue + goroutine + emitter.
type ruleWorker struct {
	rule    Rule
	meta    Meta
	deps    Deps
	emit    Emitter
	queue   chan Trigger
	metrics *scribemetrics.Registry
}

func newRuleWorker(r Rule, deps Deps, rc RuleConfig, w writerSubmitter, m *scribemetrics.Registry, queueSize int) *ruleWorker {
	deps.Config = rc
	emit := newEmitter(r.Meta(), deps.ClusterID, w, m, deps.Now)
	return &ruleWorker{
		rule:    r,
		meta:    r.Meta(),
		deps:    deps,
		emit:    emit,
		queue:   make(chan Trigger, queueSize),
		metrics: m,
	}
}

func (w *ruleWorker) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-w.queue:
			w.evaluateOne(ctx, t)
		}
	}
}

func (w *ruleWorker) evaluateOne(ctx context.Context, t Trigger) {
	defer func() {
		if r := recover(); r != nil {
			if w.metrics != nil && w.metrics.AnalysisPanics != nil {
				w.metrics.AnalysisPanics.WithLabelValues(w.meta.Kind()).Inc()
			}
			slog.Error("analysis: rule panic recovered",
				"rule", w.meta.Kind(), "panic", r, "stack", string(debug.Stack()))
		}
	}()
	var start time.Time
	if w.metrics != nil && w.metrics.AnalysisEvalDuration != nil {
		start = time.Now()
	}
	w.rule.Evaluate(ctx, t, w.deps, w.emit)
	if !start.IsZero() {
		w.metrics.AnalysisEvalDuration.WithLabelValues(w.meta.Kind()).Observe(time.Since(start).Seconds())
	}
}

func (w *ruleWorker) tickLoop(ctx context.Context) {
	tk := time.NewTicker(w.meta.TickPeriod)
	defer tk.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-tk.C:
			select {
			case w.queue <- Trigger{Tick: t}:
			default:
				if w.metrics != nil && w.metrics.AnalysisQueueDrops != nil {
					w.metrics.AnalysisQueueDrops.WithLabelValues(w.meta.Kind()).Inc()
				}
			}
		}
	}
}
