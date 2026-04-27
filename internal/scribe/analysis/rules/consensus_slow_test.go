package rules

import (
	"context"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/analysis"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

func TestConsensusSlowOpensWhenP95ExceedsThreshold(t *testing.T) {
	r := &ConsensusSlowRule{}
	cfg, _ := analysis.NewRuleConfig(r.Meta(), nil)
	deps := analysis.Deps{Store: nil, ClusterID: "c1", Now: time.Now, Config: cfg}
	var emitted []analysis.Diagnostic
	emit := func(d analysis.Diagnostic) { emitted = append(emitted, d) }

	// Push 50 commits with steadily-rising inter-block gaps (i*200ms each).
	// gap[0]=0, gap[49]=9.8s — p95 climbs past the 5s default threshold.
	tcur := time.Now().UTC()
	for i := 0; i < 50; i++ {
		gap := time.Duration(i*200) * time.Millisecond
		tcur = tcur.Add(gap)
		ev := &types.Event{
			Kind: "chain.block_committed", ClusterID: "c1", Subject: types.SubjectChain,
			Time:    tcur,
			Payload: map[string]any{"height": int64(i + 1)},
		}
		r.Evaluate(context.Background(), analysis.Trigger{Event: ev}, deps, emit)
	}
	if len(emitted) == 0 {
		t.Fatalf("no emission; expected at least one open as p95 climbed past 5s")
	}
	first := emitted[0]
	if first.State != analysis.StateOpen {
		t.Errorf("first emission state = %q, want open", first.State)
	}
	if first.Payload["recovery_key"] != "c1" {
		t.Errorf("recovery_key = %v", first.Payload["recovery_key"])
	}
}
