package analysis

import (
	"context"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

func TestSeverityAndStateConstants(t *testing.T) {
	if SeverityWarning != "warning" || SeverityError != "error" || SeverityCritical != "critical" {
		t.Errorf("unexpected severity values: %s/%s/%s", SeverityWarning, SeverityError, SeverityCritical)
	}
	if StateOpen != "open" || StateRecovered != "recovered" {
		t.Errorf("unexpected state values: %s/%s", StateOpen, StateRecovered)
	}
}

func TestKindFromMeta(t *testing.T) {
	m := Meta{Code: "block_missed", Version: 1}
	if got := m.Kind(); got != "diagnostic.block_missed_v1" {
		t.Errorf("Kind() = %q, want diagnostic.block_missed_v1", got)
	}
}

func TestRuleSignature(t *testing.T) {
	// Compile-time check that a basic Rule implementation satisfies the interface.
	var _ Rule = (*fakeRule)(nil)

	r := &fakeRule{}
	got := r.Meta()
	if got.Code != "fake" {
		t.Errorf("fake meta: %+v", got)
	}

	// Evaluate must accept Trigger / Deps / Emitter without panic on a no-op rule.
	emitted := 0
	emit := func(Diagnostic) { emitted++ }
	r.Evaluate(nil, Trigger{Tick: time.Now()}, Deps{ClusterID: "c1"}, emit)
	if emitted != 0 {
		t.Errorf("no-op rule emitted %d", emitted)
	}
}

type fakeRule struct{}

func (fakeRule) Meta() Meta {
	return Meta{Code: "fake", Version: 1, Severity: SeverityWarning, Kinds: []string{"validator.*"}}
}
func (fakeRule) Evaluate(_ context.Context, _ Trigger, _ Deps, _ Emitter) {}

// Compile-time check that Diagnostic / SignalLink fields exist for use by rules.
var _ = Diagnostic{
	Subject: "x", Severity: SeverityError, State: StateOpen,
	Recovers: "01...", Payload: map[string]any{}, LinkedSignals: []types.SignalLink{},
}
