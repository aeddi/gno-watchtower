package analysis

import (
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/scribemetrics"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

type fakeWriter struct{ submitted []types.Op }

func (w *fakeWriter) Submit(op types.Op) { w.submitted = append(w.submitted, op) }

func TestEmitterWrapsDiagnosticAsEvent(t *testing.T) {
	w := &fakeWriter{}
	m := scribemetrics.New()
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	meta := Meta{Code: "block_missed", Version: 1, Severity: SeverityWarning}
	emit := newEmitter(meta, "c1", w, m, func() time.Time { return now })

	emit(Diagnostic{
		Subject:       "moul-3",
		Payload:       map[string]any{"height": int64(100)},
		LinkedSignals: []types.SignalLink{{Type: "loki", Query: `{validator="moul-3"}`}},
	})

	if len(w.submitted) != 1 {
		t.Fatalf("submitted %d, want 1", len(w.submitted))
	}
	op := w.submitted[0]
	if op.Kind != types.OpInsertEvent || op.Event == nil {
		t.Fatalf("op kind=%v event=%v", op.Kind, op.Event)
	}
	ev := op.Event
	if ev.Kind != "diagnostic.block_missed_v1" {
		t.Errorf("kind = %q, want diagnostic.block_missed_v1", ev.Kind)
	}
	if ev.Subject != "moul-3" {
		t.Errorf("subject = %q, want moul-3", ev.Subject)
	}
	if ev.ClusterID != "c1" {
		t.Errorf("cluster = %q, want c1", ev.ClusterID)
	}
	if ev.Severity != "warning" {
		t.Errorf("severity = %q, want warning", ev.Severity)
	}
	if ev.State != "open" {
		t.Errorf("state = %q, want open (default)", ev.State)
	}
	if ev.Provenance.Type != types.ProvenanceRule {
		t.Errorf("prov type = %q, want rule", ev.Provenance.Type)
	}
	if ev.Provenance.Rule != "diagnostic.block_missed_v1" {
		t.Errorf("prov.rule = %q", ev.Provenance.Rule)
	}
	if ev.Provenance.DocRef != "/docs/rules/diagnostic.block_missed_v1" {
		t.Errorf("prov.doc_ref = %q", ev.Provenance.DocRef)
	}
	if len(ev.Provenance.LinkedSignals) != 1 {
		t.Errorf("linked_signals = %v", ev.Provenance.LinkedSignals)
	}
}

func TestEmitterUsesPerEmissionSeverityOverride(t *testing.T) {
	w := &fakeWriter{}
	m := scribemetrics.New()
	meta := Meta{Code: "x", Version: 1, Severity: SeverityWarning}
	emit := newEmitter(meta, "c1", w, m, time.Now)
	emit(Diagnostic{Subject: "_chain", Severity: SeverityCritical})
	if got := w.submitted[0].Event.Severity; got != "critical" {
		t.Errorf("severity = %q, want critical", got)
	}
}

func TestEmitterDropsRecoveredWithoutRecoversID(t *testing.T) {
	w := &fakeWriter{}
	m := scribemetrics.New()
	meta := Meta{Code: "x", Version: 1, Severity: SeverityWarning}
	emit := newEmitter(meta, "c1", w, m, time.Now)
	emit(Diagnostic{Subject: "x", State: StateRecovered, Recovers: ""})
	if len(w.submitted) != 0 {
		t.Errorf("expected drop, got %d submitted", len(w.submitted))
	}
}

func TestEmitterIncrementsEmissionsCounter(t *testing.T) {
	w := &fakeWriter{}
	m := scribemetrics.New()
	meta := Meta{Code: "x", Version: 1, Severity: SeverityWarning}
	emit := newEmitter(meta, "c1", w, m, time.Now)
	emit(Diagnostic{Subject: "_chain"})

	mfs, _ := m.Registry.Gather()
	var n float64
	for _, mf := range mfs {
		if mf.GetName() == "scribe_analysis_emissions_total" {
			for _, mtr := range mf.GetMetric() {
				n += mtr.GetCounter().GetValue()
			}
		}
	}
	if n != 1 {
		t.Errorf("emissions counter = %f, want 1", n)
	}
}
