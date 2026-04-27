package rules

import (
	"context"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/analysis"
	"github.com/aeddi/gno-watchtower/internal/scribe/store"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

type chainSampleStore struct {
	store.Store
	chain *types.SampleChain
}

func (s chainSampleStore) GetLatestSampleChain(_ context.Context, _ string, _ time.Time) (*types.SampleChain, error) {
	return s.chain, nil
}

func TestBFTAtRiskOpensWhenOnlinePowerBelowThreshold(t *testing.T) {
	r := &BFTAtRiskRule{}
	cfg, _ := analysis.NewRuleConfig(r.Meta(), nil)
	st := chainSampleStore{chain: &types.SampleChain{
		ClusterID: "c1", TotalVotingPower: 300, OnlineCount: 1, ValsetSize: 3,
		// 1 of 3 validators online → 66.67% offline → above 33.33% threshold.
	}}
	deps := analysis.Deps{Store: st, ClusterID: "c1", Now: time.Now, Config: cfg}

	var emitted []analysis.Diagnostic
	emit := func(d analysis.Diagnostic) { emitted = append(emitted, d) }
	trig := analysis.Trigger{Event: &types.Event{Kind: "validator.went_offline", ClusterID: "c1"}}
	r.Evaluate(context.Background(), trig, deps, emit)

	if len(emitted) != 1 || emitted[0].State != analysis.StateOpen {
		t.Fatalf("expected 1 open emission, got %+v", emitted)
	}
	if emitted[0].Payload["recovery_key"] != "c1" {
		t.Errorf("recovery_key missing: %+v", emitted[0].Payload)
	}
}

func TestBFTAtRiskDoesNotReOpenWhileStillOpen(t *testing.T) {
	r := &BFTAtRiskRule{}
	cfg, _ := analysis.NewRuleConfig(r.Meta(), nil)
	st := chainSampleStore{chain: &types.SampleChain{
		ClusterID: "c1", TotalVotingPower: 300, OnlineCount: 1, ValsetSize: 3,
	}}
	deps := analysis.Deps{Store: st, ClusterID: "c1", Now: time.Now, Config: cfg}
	var n int
	emit := func(analysis.Diagnostic) { n++ }
	trig := analysis.Trigger{Event: &types.Event{Kind: "validator.went_offline", ClusterID: "c1"}}
	r.Evaluate(context.Background(), trig, deps, emit)
	r.Evaluate(context.Background(), trig, deps, emit)
	if n != 1 {
		t.Errorf("emitted %d times, want 1 (idempotent open)", n)
	}
}

func TestBFTAtRiskRecoversWhenPowerReturns(t *testing.T) {
	r := &BFTAtRiskRule{}
	cfg, _ := analysis.NewRuleConfig(r.Meta(), nil)
	st := &chainSampleStore{chain: &types.SampleChain{
		ClusterID: "c1", TotalVotingPower: 300, OnlineCount: 1, ValsetSize: 3,
	}}
	deps := analysis.Deps{Store: st, ClusterID: "c1", Now: time.Now, Config: cfg}
	var emitted []analysis.Diagnostic
	emit := func(d analysis.Diagnostic) { emitted = append(emitted, d) }

	r.Evaluate(context.Background(),
		analysis.Trigger{Event: &types.Event{Kind: "validator.went_offline", ClusterID: "c1"}}, deps, emit)
	st.chain.OnlineCount = 3 // ratio recovered
	r.Evaluate(context.Background(),
		analysis.Trigger{Event: &types.Event{Kind: "validator.came_online", ClusterID: "c1"}}, deps, emit)

	if len(emitted) != 2 {
		t.Fatalf("emitted %d, want 2", len(emitted))
	}
	if emitted[1].State != analysis.StateRecovered || emitted[1].Recovers == "" {
		t.Errorf("second emission not a proper recovered: %+v", emitted[1])
	}
}
