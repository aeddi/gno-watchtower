package rules

import (
	"context"
	_ "embed"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/analysis"
	"github.com/aeddi/gno-watchtower/internal/scribe/store"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

//go:embed block_missed.md
var blockMissedDoc string

// BlockMissedRule fires per (validator, height) for any active validator that
// did not produce a validator.vote_cast{height=H} event for a committed
// height H. Point-in-time rule (no recovery semantics).
type BlockMissedRule struct {
	// validators is the static set of validators we expect to vote. v1 reads
	// this from cluster config injected at registration time; later versions
	// may resolve dynamically per height. Set via the package-level
	// SetBlockMissedValidators helper if needed.
	validators []string
}

// Meta returns the rule descriptor used by the engine and /api/rules.
func (r *BlockMissedRule) Meta() analysis.Meta {
	return analysis.Meta{
		Code:        "block_missed",
		Version:     1,
		Severity:    analysis.SeverityWarning,
		Kinds:       []string{"chain.block_committed"},
		Description: "Validator did not vote at a committed height.",
	}
}

// Evaluate checks each expected validator's recent vote_cast history for the
// just-committed height and emits a diagnostic for those that didn't vote.
func (r *BlockMissedRule) Evaluate(ctx context.Context, t analysis.Trigger, d analysis.Deps, emit analysis.Emitter) {
	if t.Event == nil || t.Event.Kind != "chain.block_committed" {
		return
	}
	height, _ := t.Event.Payload["height"].(int64)
	if height == 0 {
		return
	}
	for _, val := range r.validators {
		evs, _, err := d.Store.QueryEvents(ctx, store.EventQuery{
			ClusterID: d.ClusterID, Subject: val, Kind: "validator.vote_cast",
			Limit: 50,
		})
		if err != nil {
			continue
		}
		voted := false
		for _, e := range evs {
			if h, _ := e.Payload["height"].(int64); h == height {
				voted = true
				break
			}
		}
		if voted {
			continue
		}
		emit(analysis.Diagnostic{
			Subject: val,
			Payload: map[string]any{
				"height":            height,
				"missing_validator": val,
			},
			LinkedSignals: []types.SignalLink{{
				Type:  "loki",
				Query: `{validator="` + val + `"} |~ "received complete proposal block"`,
				From:  t.Event.Time.Add(-30 * time.Second),
				To:    t.Event.Time.Add(30 * time.Second),
			}},
		})
	}
}

func init() {
	// validators slice is empty at static registration. The empty-validator
	// rule is a no-op at runtime, which is the right default for scribe
	// instances that don't know the validator set yet. A future task may
	// inject the set from cluster config.
	analysis.Register(&BlockMissedRule{}, blockMissedDoc)
}
