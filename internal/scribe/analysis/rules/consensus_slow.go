package rules

import (
	"context"
	_ "embed"
	"sort"
	"sync"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/analysis"
	"github.com/aeddi/gno-watchtower/internal/scribe/eventid"
)

//go:embed consensus_slow.md
var consensusSlowDoc string

const consensusSlowWindow = 50

// ConsensusSlowRule maintains a rolling window of inter-block durations and
// emits when p95 exceeds slow_threshold_seconds. Recovery-tracking.
type ConsensusSlowRule struct {
	tracker analysis.Tracker

	mu       sync.Mutex
	window   []time.Duration
	lastTime time.Time
}

// Meta returns the rule descriptor used by the engine and /api/rules.
func (r *ConsensusSlowRule) Meta() analysis.Meta {
	return analysis.Meta{
		Code:        "consensus_slow",
		Version:     1,
		Severity:    analysis.SeverityWarning,
		Kinds:       []string{"chain.block_committed"},
		Description: "Inter-block p95 duration exceeds threshold.",
		Params: map[string]analysis.ParamSpec{
			"slow_threshold_seconds": {Default: 5.0, Min: 0.1, Max: 600.0},
		},
	}
}

// RecoveryTracker exposes the rule's recovery state to the engine for rehydration.
func (r *ConsensusSlowRule) RecoveryTracker() *analysis.Tracker { return &r.tracker }

// Evaluate appends the inter-block gap to the rolling window and emits open
// or recovered when p95 crosses the threshold.
func (r *ConsensusSlowRule) Evaluate(_ context.Context, t analysis.Trigger, d analysis.Deps, emit analysis.Emitter) {
	if t.Event == nil || t.Event.Kind != "chain.block_committed" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.lastTime.IsZero() {
		gap := t.Event.Time.Sub(r.lastTime)
		if gap > 0 {
			r.window = append(r.window, gap)
			if len(r.window) > consensusSlowWindow {
				r.window = r.window[len(r.window)-consensusSlowWindow:]
			}
		}
	}
	r.lastTime = t.Event.Time

	if len(r.window) < 5 {
		return // not enough samples yet
	}
	p95 := percentileDuration(r.window, 0.95)
	threshold := time.Duration(d.Config.Float64("slow_threshold_seconds") * float64(time.Second))
	key := d.ClusterID

	if p95 >= threshold {
		eid := eventid.Derive(d.Now(), r.Meta().Kind(), key, []byte(key+"open"))
		if r.tracker.Open(key, eid) {
			emit(analysis.Diagnostic{
				Subject: "_chain",
				State:   analysis.StateOpen,
				Payload: map[string]any{
					"recovery_key":      key,
					"p95_seconds":       p95.Seconds(),
					"threshold_seconds": threshold.Seconds(),
					"window_size":       len(r.window),
				},
			})
		}
		return
	}
	if openID := r.tracker.Recovered(key); openID != "" {
		emit(analysis.Diagnostic{
			Subject:  "_chain",
			State:    analysis.StateRecovered,
			Recovers: openID,
			Payload: map[string]any{
				"recovery_key":      key,
				"p95_seconds":       p95.Seconds(),
				"threshold_seconds": threshold.Seconds(),
			},
		})
	}
}

// percentileDuration returns the p-th percentile of a duration slice.
func percentileDuration(in []time.Duration, p float64) time.Duration {
	if len(in) == 0 {
		return 0
	}
	cp := append([]time.Duration{}, in...)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	idx := int(float64(len(cp)) * p)
	if idx >= len(cp) {
		idx = len(cp) - 1
	}
	return cp[idx]
}

func init() {
	analysis.Register(&ConsensusSlowRule{}, consensusSlowDoc)
}
