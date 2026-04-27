package rules

import (
	"context"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/analysis"
	"github.com/aeddi/gno-watchtower/internal/scribe/store"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

type latestEventStore struct {
	store.Store
	latest *types.Event
}

func (s latestEventStore) QueryEvents(_ context.Context, q store.EventQuery) ([]types.Event, string, error) {
	if s.latest == nil || q.Kind != "chain.block_committed" {
		return nil, "", nil
	}
	return []types.Event{*s.latest}, "", nil
}

func TestConsensusStuckOpensWhenLatestCommitIsOld(t *testing.T) {
	r := &ConsensusStuckRule{}
	cfg, _ := analysis.NewRuleConfig(r.Meta(), nil)
	now := time.Now().UTC()
	st := latestEventStore{latest: &types.Event{Time: now.Add(-90 * time.Second)}}
	deps := analysis.Deps{Store: st, ClusterID: "c1", Now: func() time.Time { return now }, Config: cfg}

	var emitted []analysis.Diagnostic
	emit := func(d analysis.Diagnostic) { emitted = append(emitted, d) }
	r.Evaluate(context.Background(), analysis.Trigger{Tick: now}, deps, emit)

	if len(emitted) != 1 || emitted[0].State != analysis.StateOpen {
		t.Fatalf("expected 1 open, got %+v", emitted)
	}
	if emitted[0].Payload["recovery_key"] != "c1" {
		t.Errorf("recovery_key = %v", emitted[0].Payload["recovery_key"])
	}
}

func TestConsensusStuckRecoversOnFreshCommit(t *testing.T) {
	r := &ConsensusStuckRule{}
	cfg, _ := analysis.NewRuleConfig(r.Meta(), nil)
	now := time.Now().UTC()
	st := &latestEventStore{latest: &types.Event{Time: now.Add(-90 * time.Second)}}
	deps := analysis.Deps{Store: st, ClusterID: "c1", Now: func() time.Time { return now }, Config: cfg}
	var emitted []analysis.Diagnostic
	emit := func(d analysis.Diagnostic) { emitted = append(emitted, d) }

	r.Evaluate(context.Background(), analysis.Trigger{Tick: now}, deps, emit)
	st.latest.Time = now.Add(-1 * time.Second)
	r.Evaluate(context.Background(), analysis.Trigger{Tick: now}, deps, emit)

	if len(emitted) != 2 || emitted[1].State != analysis.StateRecovered {
		t.Fatalf("expected open then recovered: %+v", emitted)
	}
}
