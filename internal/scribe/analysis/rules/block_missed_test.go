package rules

import (
	"context"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/analysis"
	"github.com/aeddi/gno-watchtower/internal/scribe/store"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

// fakeStore returns canned QueryEvents results keyed by (subject, kind).
type fakeStore struct {
	store.Store
	voteCasts map[string][]types.Event // key = "subject|kind"
}

func (f fakeStore) QueryEvents(_ context.Context, q store.EventQuery) ([]types.Event, string, error) {
	return f.voteCasts[q.Subject+"|"+q.Kind], "", nil
}

func TestBlockMissedEmitsForValidatorsThatDidNotVote(t *testing.T) {
	now := time.Now().UTC()
	commit := &types.Event{
		EventID: "01J0", Kind: "chain.block_committed",
		Time: now, IngestTime: now, Subject: types.SubjectChain,
		Payload: map[string]any{"height": int64(100)},
	}
	st := fakeStore{
		voteCasts: map[string][]types.Event{
			"val-A|validator.vote_cast": {{Payload: map[string]any{"height": int64(100)}}},
			"val-B|validator.vote_cast": {{Payload: map[string]any{"height": int64(99)}}},
		},
	}
	r := &BlockMissedRule{validators: []string{"val-A", "val-B"}}
	cfg, _ := analysis.NewRuleConfig(r.Meta(), nil)
	deps := analysis.Deps{Store: st, ClusterID: "c1", Now: time.Now, Config: cfg}

	var emitted []analysis.Diagnostic
	emit := func(d analysis.Diagnostic) { emitted = append(emitted, d) }
	r.Evaluate(context.Background(), analysis.Trigger{Event: commit}, deps, emit)

	if len(emitted) != 1 {
		t.Fatalf("emitted %d, want 1: %+v", len(emitted), emitted)
	}
	if emitted[0].Subject != "val-B" {
		t.Errorf("subject = %q, want val-B", emitted[0].Subject)
	}
	if h, _ := emitted[0].Payload["height"].(int64); h != 100 {
		t.Errorf("payload.height = %v, want 100", emitted[0].Payload["height"])
	}
}

func TestBlockMissedSkipsNonCommitTriggers(t *testing.T) {
	r := &BlockMissedRule{validators: []string{"val-A"}}
	cfg, _ := analysis.NewRuleConfig(r.Meta(), nil)
	deps := analysis.Deps{ClusterID: "c1", Now: time.Now, Config: cfg}
	var emitted int
	emit := func(analysis.Diagnostic) { emitted++ }
	r.Evaluate(context.Background(),
		analysis.Trigger{Event: &types.Event{Kind: "validator.vote_cast"}},
		deps, emit)
	if emitted != 0 {
		t.Errorf("emitted %d on non-commit trigger", emitted)
	}
}
