package analysis

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/scribemetrics"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

// chanWriter is a writerSubmitter that forwards Ops to a channel for tests.
type chanWriter struct{ ch chan types.Op }

func (w *chanWriter) Submit(op types.Op) { w.ch <- op }

// stubBroadcaster mimics writer.Subscribe for tests — returns a channel the
// test pushes to; the engine subscribes to it.
type stubBroadcaster struct{ events chan types.Event }

func (s *stubBroadcaster) Subscribe(_ int) chan types.Event { return s.events }
func (s *stubBroadcaster) Unsubscribe(_ chan types.Event)   {}

// echoRule emits one diagnostic per Trigger.Event seen.
type echoRule struct{ count atomic.Int64 }

func (r *echoRule) Meta() Meta {
	return Meta{Code: "echo", Version: 1, Severity: SeverityWarning, Kinds: []string{"x.*"}}
}

func (r *echoRule) Evaluate(_ context.Context, t Trigger, _ Deps, emit Emitter) {
	if t.Event == nil {
		return
	}
	r.count.Add(1)
	emit(Diagnostic{Subject: "test", Payload: map[string]any{"seen": t.Event.EventID}})
}

func TestEngineDispatchesEventsThatMatchKinds(t *testing.T) {
	resetRegistryForTest(t)
	rule := &echoRule{}
	Register(rule, "doc")

	bc := &stubBroadcaster{events: make(chan types.Event, 8)}
	cw := &chanWriter{ch: make(chan types.Op, 8)}
	m := scribemetrics.New()

	eng, err := New(Deps{ClusterID: "c1", Now: time.Now}, bc, cw, m, EngineConfig{QueueSize: 4})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := eng.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	bc.events <- types.Event{EventID: "01J0", Kind: "x.foo", ClusterID: "c1"}
	bc.events <- types.Event{EventID: "01J1", Kind: "y.foo", ClusterID: "c1"} // ignored
	bc.events <- types.Event{EventID: "01J2", Kind: "x.bar", ClusterID: "c1"}

	deadline := time.After(2 * time.Second)
	got := 0
	for got < 2 {
		select {
		case <-cw.ch:
			got++
		case <-deadline:
			t.Fatalf("timed out after %d emissions (want 2)", got)
		}
	}
	if rule.count.Load() != 2 {
		t.Errorf("rule.count = %d, want 2", rule.count.Load())
	}
}

// panicRule panics once on the first event, then stays quiet.
type panicRule struct{ count atomic.Int64 }

func (r *panicRule) Meta() Meta {
	return Meta{Code: "panic", Version: 1, Severity: SeverityWarning, Kinds: []string{"x.*"}}
}

func (r *panicRule) Evaluate(_ context.Context, _ Trigger, _ Deps, _ Emitter) {
	r.count.Add(1)
	if r.count.Load() == 1 {
		panic("boom")
	}
}

func TestEnginePanicInRuleDoesNotKillWorker(t *testing.T) {
	resetRegistryForTest(t)
	r := &panicRule{}
	Register(r, "doc")
	bc := &stubBroadcaster{events: make(chan types.Event, 8)}
	cw := &chanWriter{ch: make(chan types.Op, 1)}
	m := scribemetrics.New()
	eng, _ := New(Deps{ClusterID: "c1", Now: time.Now}, bc, cw, m, EngineConfig{QueueSize: 4})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = eng.Start(ctx)

	bc.events <- types.Event{EventID: "01J0", Kind: "x.a"}
	bc.events <- types.Event{EventID: "01J1", Kind: "x.b"}

	deadline := time.After(2 * time.Second)
	for r.count.Load() < 2 {
		select {
		case <-deadline:
			t.Fatalf("worker stalled after panic; count = %d", r.count.Load())
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// slowRule sleeps so its queue can overflow.
type slowRule struct{ wake chan struct{} }

func (r *slowRule) Meta() Meta {
	return Meta{Code: "slow", Version: 1, Severity: SeverityWarning, Kinds: []string{"x.*"}}
}

func (r *slowRule) Evaluate(_ context.Context, _ Trigger, _ Deps, _ Emitter) {
	<-r.wake
}

func TestEngineQueueOverflowIncrementsDropCounter(t *testing.T) {
	resetRegistryForTest(t)
	r := &slowRule{wake: make(chan struct{})}
	Register(r, "doc")
	bc := &stubBroadcaster{events: make(chan types.Event, 32)}
	cw := &chanWriter{ch: make(chan types.Op, 1)}
	m := scribemetrics.New()
	eng, _ := New(Deps{ClusterID: "c1", Now: time.Now}, bc, cw, m, EngineConfig{QueueSize: 2})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = eng.Start(ctx)

	for i := 0; i < 20; i++ {
		bc.events <- types.Event{EventID: "01J", Kind: "x.foo"}
	}

	time.Sleep(200 * time.Millisecond)

	mfs, _ := m.Registry.Gather()
	var drops float64
	for _, mf := range mfs {
		if mf.GetName() == "scribe_analysis_queue_drops_total" {
			for _, mtr := range mf.GetMetric() {
				drops += mtr.GetCounter().GetValue()
			}
		}
	}
	if drops == 0 {
		t.Errorf("expected queue drops, got 0")
	}
	close(r.wake)
}

// tickRule emits on every tick. Used to verify the slow-tick fallback fires.
type tickRule struct{ count atomic.Int64 }

func (r *tickRule) Meta() Meta {
	return Meta{Code: "tick", Version: 1, Severity: SeverityWarning, TickPeriod: 30 * time.Millisecond}
}

func (r *tickRule) Evaluate(_ context.Context, t Trigger, _ Deps, emit Emitter) {
	if t.Tick.IsZero() {
		return
	}
	r.count.Add(1)
	emit(Diagnostic{Subject: "_chain"})
}

func TestEngineTickRuleFiresOnSchedule(t *testing.T) {
	resetRegistryForTest(t)
	r := &tickRule{}
	Register(r, "doc")
	bc := &stubBroadcaster{events: make(chan types.Event, 1)}
	cw := &chanWriter{ch: make(chan types.Op, 8)}
	m := scribemetrics.New()
	eng, _ := New(Deps{ClusterID: "c1", Now: time.Now}, bc, cw, m, EngineConfig{QueueSize: 4})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_ = eng.Start(ctx)
	<-ctx.Done()
	time.Sleep(50 * time.Millisecond) // let any final tick drain

	if r.count.Load() < 2 {
		t.Errorf("tick fired %d times; want >= 2 over 200ms with 30ms period", r.count.Load())
	}
}
