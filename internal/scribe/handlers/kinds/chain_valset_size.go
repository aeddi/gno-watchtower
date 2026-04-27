package kinds

import (
	"context"
	_ "embed"
	"sync"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/handlers"
	"github.com/aeddi/gno-watchtower/internal/scribe/normalizer"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

//go:embed chain_valset_size.md
var chainValsetSizeDoc string

// ValsetSize watches sentinel_rpc_validator_set_power and aggregates per poll-tick
// (by Metric.Time). When a new tick arrives it flushes the prior tick's aggregate
// as a samples_chain upsert with ValsetSize = count and TotalVotingPower = sum.
type ValsetSize struct {
	cluster string
	mu      sync.Mutex
	pending map[time.Time]*valsetAccum
}

type valsetAccum struct {
	count int
	total int64
}

func NewValsetSize(cluster string) *ValsetSize {
	return &ValsetSize{cluster: cluster, pending: map[time.Time]*valsetAccum{}}
}

func (v *ValsetSize) Name() string { return "valset_size" }

// Meta returns the descriptor used by the handler registry and /api/handlers.
// ValsetSize only upserts samples_chain (valset_size, total_voting_power); no event.
func (v *ValsetSize) Meta() handlers.Meta {
	return handlers.Meta{
		Kind:        "chain.valset_size",
		Source:      handlers.SourceMetric,
		Description: "Validator set size and total voting power per poll tick (sample upsert only, no event).",
		DocRef:      "/docs/handlers/chain.valset_size",
	}
}

func (v *ValsetSize) Handle(_ context.Context, o normalizer.Observation) []types.Op {
	if o.Metric == nil || o.MetricQuery != "sentinel_rpc_validator_set_power" {
		return nil
	}
	t := o.Metric.Time

	v.mu.Lock()
	defer v.mu.Unlock()

	// Flush any prior ticks (Time < t) — they're complete now.
	var ops []types.Op
	for past, acc := range v.pending {
		if past.Before(t) {
			ops = append(ops, types.Op{Kind: types.OpUpsertSampleChain, SampleChain: &types.SampleChain{
				ClusterID:        v.cluster,
				Time:             past,
				Tier:             0,
				ValsetSize:       int16(acc.count),
				TotalVotingPower: acc.total,
			}})
			delete(v.pending, past)
		}
	}

	// Accumulate the current observation.
	acc, ok := v.pending[t]
	if !ok {
		acc = &valsetAccum{}
		v.pending[t] = acc
	}
	acc.count++
	acc.total += int64(o.Metric.Value)
	return ops
}

func init() {
	handlers.Register("chain.valset_size",
		func(cluster string) handlers.Handler { return NewValsetSize(cluster) },
		chainValsetSizeDoc)
}
