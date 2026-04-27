package rules

import (
	"context"
	_ "embed"

	"github.com/aeddi/gno-watchtower/internal/scribe/analysis"
	"github.com/aeddi/gno-watchtower/internal/scribe/eventid"
)

//go:embed bft_at_risk.md
var bftAtRiskDoc string

// BFTAtRiskRule emits state=open when the chain's online-validator fraction
// drops below the BFT threshold (default 33.33% offline = 2/3 online), and
// state=recovered when it returns above. Recovery key is the cluster ID
// (one open incident per cluster).
type BFTAtRiskRule struct {
	tracker analysis.Tracker
}

// Meta returns the rule descriptor used by the engine and /api/rules.
func (r *BFTAtRiskRule) Meta() analysis.Meta {
	return analysis.Meta{
		Code:        "bft_at_risk",
		Version:     1,
		Severity:    analysis.SeverityError,
		Kinds:       []string{"validator.went_offline", "validator.came_online"},
		Description: "Online voting power below BFT threshold (chain at risk of halting).",
		Params: map[string]analysis.ParamSpec{
			"voting_power_threshold_pct": {Default: 33.33, Min: 0.0, Max: 50.0},
		},
	}
}

// RecoveryTracker exposes the rule's recovery state to the engine for
// rehydration. Required by all rules that opt into open/recovered semantics.
func (r *BFTAtRiskRule) RecoveryTracker() *analysis.Tracker { return &r.tracker }

// Evaluate computes the offline-fraction from the latest chain sample and
// transitions the recovery tracker, emitting state=open or state=recovered
// on transitions.
func (r *BFTAtRiskRule) Evaluate(ctx context.Context, t analysis.Trigger, d analysis.Deps, emit analysis.Emitter) {
	if t.Event == nil {
		return
	}
	chain, err := d.Store.GetLatestSampleChain(ctx, d.ClusterID, d.Now())
	if err != nil || chain == nil || chain.ValsetSize == 0 {
		return
	}
	// v1 proxy: online_count / valset_size as a stand-in for online voting
	// power fraction. Future v2 may compute true online voting power by
	// joining per-validator samples.
	offlineFrac := 100.0 * float64(chain.ValsetSize-chain.OnlineCount) / float64(chain.ValsetSize)
	threshold := d.Config.Float64("voting_power_threshold_pct")

	key := d.ClusterID
	if offlineFrac >= threshold {
		eid := eventid.Derive(d.Now(), r.Meta().Kind(), key, []byte(key+"open"))
		if r.tracker.Open(key, eid) {
			emit(analysis.Diagnostic{
				Subject: "_chain",
				State:   analysis.StateOpen,
				Payload: map[string]any{
					"recovery_key":     key,
					"online_count":     chain.OnlineCount,
					"valset_size":      chain.ValsetSize,
					"offline_fraction": offlineFrac,
					"threshold_pct":    threshold,
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
				"recovery_key":     key,
				"online_count":     chain.OnlineCount,
				"valset_size":      chain.ValsetSize,
				"offline_fraction": offlineFrac,
			},
		})
	}
}

func init() {
	analysis.Register(&BFTAtRiskRule{}, bftAtRiskDoc)
}
