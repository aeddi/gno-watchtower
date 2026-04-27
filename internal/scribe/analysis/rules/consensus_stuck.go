package rules

import (
	"context"
	_ "embed"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/analysis"
	"github.com/aeddi/gno-watchtower/internal/scribe/eventid"
	"github.com/aeddi/gno-watchtower/internal/scribe/store"
)

//go:embed consensus_stuck.md
var consensusStuckDoc string

// ConsensusStuckRule fires when the chain has not produced a block in the
// configured threshold_seconds. Pure tick rule (no event subscription).
type ConsensusStuckRule struct {
	tracker analysis.Tracker
}

// Meta returns the rule descriptor used by the engine and /api/rules.
func (r *ConsensusStuckRule) Meta() analysis.Meta {
	return analysis.Meta{
		Code:        "consensus_stuck",
		Version:     1,
		Severity:    analysis.SeverityError,
		Kinds:       nil, // tick-only
		TickPeriod:  15 * time.Second,
		Description: "No new block committed in threshold_seconds (chain stuck).",
		Params: map[string]analysis.ParamSpec{
			"threshold_seconds": {Default: int64(60), Min: int64(5), Max: int64(86400)},
		},
	}
}

// RecoveryTracker exposes the rule's recovery state to the engine for rehydration.
func (r *ConsensusStuckRule) RecoveryTracker() *analysis.Tracker { return &r.tracker }

// Evaluate reads the latest chain.block_committed event and transitions the
// recovery tracker based on its age relative to threshold_seconds.
func (r *ConsensusStuckRule) Evaluate(ctx context.Context, t analysis.Trigger, d analysis.Deps, emit analysis.Emitter) {
	if t.Tick.IsZero() {
		return
	}
	evs, _, err := d.Store.QueryEvents(ctx, store.EventQuery{
		ClusterID: d.ClusterID, Kind: "chain.block_committed", Limit: 1,
	})
	if err != nil {
		return
	}
	now := d.Now()
	threshold := time.Duration(d.Config.Int("threshold_seconds")) * time.Second
	key := d.ClusterID

	if len(evs) == 0 || now.Sub(evs[0].Time) > threshold {
		eid := eventid.Derive(now, r.Meta().Kind(), key, []byte(key+"open"))
		if r.tracker.Open(key, eid) {
			payload := map[string]any{
				"recovery_key":      key,
				"threshold_seconds": int64(threshold.Seconds()),
			}
			if len(evs) > 0 {
				payload["last_block_time"] = evs[0].Time
				payload["age_seconds"] = now.Sub(evs[0].Time).Seconds()
			}
			emit(analysis.Diagnostic{Subject: "_chain", State: analysis.StateOpen, Payload: payload})
		}
		return
	}
	if openID := r.tracker.Recovered(key); openID != "" {
		emit(analysis.Diagnostic{
			Subject:  "_chain",
			State:    analysis.StateRecovered,
			Recovers: openID,
			Payload: map[string]any{
				"recovery_key":    key,
				"last_block_time": evs[0].Time,
			},
		})
	}
}

func init() {
	analysis.Register(&ConsensusStuckRule{}, consensusStuckDoc)
}
