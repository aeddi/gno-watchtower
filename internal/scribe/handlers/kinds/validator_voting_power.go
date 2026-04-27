package kinds

import (
	"context"
	_ "embed"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/handlers"
	"github.com/aeddi/gno-watchtower/internal/scribe/normalizer"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

//go:embed validator_voting_power.md
var validatorVotingPowerDoc string

// VotingPower watches sentinel_rpc_validator_voting_power and upserts voting_power samples.
type VotingPower struct {
	cluster string
}

func NewVotingPower(cluster string) *VotingPower { return &VotingPower{cluster: cluster} }

func (VotingPower) Name() string { return "voting_power" }

// Meta returns the descriptor used by the handler registry and /api/handlers.
// VotingPower only upserts samples_validator (voting_power); it does not emit events.
func (VotingPower) Meta() handlers.Meta {
	return handlers.Meta{
		Kind:        "validator.voting_power",
		Source:      handlers.SourceMetric,
		Description: "Voting power per validator (sample upsert only, no event).",
		DocRef:      "/docs/handlers/validator.voting_power",
	}
}

func (h *VotingPower) Handle(_ context.Context, o normalizer.Observation) []types.Op {
	if o.Metric == nil || o.MetricQuery != "sentinel_rpc_validator_voting_power" {
		return nil
	}
	val := o.Metric.Labels["validator"]
	// +5ns offset avoids PK collision with sibling handlers.
	sv := types.SampleValidator{
		ClusterID: h.cluster, Validator: val, Time: o.Metric.Time.Add(5 * time.Microsecond), Tier: 0,
		VotingPower: int64(o.Metric.Value),
	}
	return []types.Op{{Kind: types.OpUpsertSampleValidator, SampleValid: &sv}}
}

func init() {
	handlers.Register("validator.voting_power",
		func(cluster string) handlers.Handler { return NewVotingPower(cluster) },
		validatorVotingPowerDoc)
}
