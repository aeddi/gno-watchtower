package rules

import (
	"context"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/analysis"
	"github.com/aeddi/gno-watchtower/internal/scribe/store"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

type subjectsAndSampleStore struct {
	store.Store
	subjects []string
	sample   *types.SampleValidator
}

func (s subjectsAndSampleStore) ListSubjects(_ context.Context, _ string) ([]string, error) {
	return s.subjects, nil
}

func (s subjectsAndSampleStore) GetMergedSampleValidator(_ context.Context, _, _ string, _ time.Time, _ time.Duration) (*types.SampleValidator, error) {
	return s.sample, nil
}

func TestValidatorIsolatedOpensAfterSustainedLowPeers(t *testing.T) {
	r := &ValidatorIsolatedRule{}
	cfg, _ := analysis.NewRuleConfig(r.Meta(), nil)
	now := time.Now().UTC()
	st := &subjectsAndSampleStore{
		subjects: []string{"val-A"},
		sample:   &types.SampleValidator{PeerCountIn: 0, PeerCountOut: 1, LastObserved: now},
	}
	deps := analysis.Deps{Store: st, ClusterID: "c1", Now: func() time.Time { return now }, Config: cfg}
	var emitted []analysis.Diagnostic
	emit := func(d analysis.Diagnostic) { emitted = append(emitted, d) }

	// First tick: low peers detected, but condition hasn't been low for long enough.
	r.Evaluate(context.Background(), analysis.Trigger{Tick: now}, deps, emit)
	if len(emitted) != 0 {
		t.Fatalf("expected no emission on first low-peer tick: %+v", emitted)
	}
	// Advance "now" past isolated_threshold_seconds (default 30s).
	now2 := now.Add(40 * time.Second)
	deps.Now = func() time.Time { return now2 }
	r.Evaluate(context.Background(), analysis.Trigger{Tick: now2}, deps, emit)

	if len(emitted) != 1 || emitted[0].State != analysis.StateOpen {
		t.Fatalf("expected open after sustained low peers: %+v", emitted)
	}
	if emitted[0].Subject != "val-A" {
		t.Errorf("subject = %q, want val-A", emitted[0].Subject)
	}
	if emitted[0].Payload["recovery_key"] != "c1|val-A" {
		t.Errorf("recovery_key = %v, want c1|val-A", emitted[0].Payload["recovery_key"])
	}
}

func TestValidatorIsolatedRecoversWhenPeersReturn(t *testing.T) {
	r := &ValidatorIsolatedRule{}
	cfg, _ := analysis.NewRuleConfig(r.Meta(), nil)
	now := time.Now().UTC()
	st := &subjectsAndSampleStore{
		subjects: []string{"val-A"},
		sample:   &types.SampleValidator{PeerCountIn: 0, PeerCountOut: 1, LastObserved: now},
	}
	deps := analysis.Deps{Store: st, ClusterID: "c1", Now: func() time.Time { return now }, Config: cfg}
	var emitted []analysis.Diagnostic
	emit := func(d analysis.Diagnostic) { emitted = append(emitted, d) }

	r.Evaluate(context.Background(), analysis.Trigger{Tick: now}, deps, emit)
	now2 := now.Add(40 * time.Second)
	deps.Now = func() time.Time { return now2 }
	r.Evaluate(context.Background(), analysis.Trigger{Tick: now2}, deps, emit)
	st.sample.PeerCountIn = 5
	now3 := now2.Add(35 * time.Second)
	deps.Now = func() time.Time { return now3 }
	r.Evaluate(context.Background(), analysis.Trigger{Tick: now3}, deps, emit)

	if len(emitted) < 2 {
		t.Fatalf("expected open then recovered: %+v", emitted)
	}
	last := emitted[len(emitted)-1]
	if last.State != analysis.StateRecovered || last.Recovers == "" {
		t.Errorf("last emission not recovered: %+v", last)
	}
}
