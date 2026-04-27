package kinds

import (
	"context"
	_ "embed"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/handlers"
	"github.com/aeddi/gno-watchtower/internal/scribe/normalizer"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

//go:embed validator_mempool.md
var validatorMempoolDoc string

// Mempool watches sentinel_rpc_mempool_txs and upserts mempool_txs samples.
type Mempool struct {
	cluster string
}

func NewMempool(cluster string) *Mempool { return &Mempool{cluster: cluster} }

func (Mempool) Name() string { return "mempool" }

// Meta returns the descriptor used by the handler registry and /api/handlers.
// Mempool only upserts samples_validator (mempool_txs); it does not emit events.
func (Mempool) Meta() handlers.Meta {
	return handlers.Meta{
		Kind:        "validator.mempool",
		Source:      handlers.SourceMetric,
		Description: "Pending mempool transaction count per validator (sample upsert only, no event).",
		DocRef:      "/docs/handlers/validator.mempool",
	}
}

func (h *Mempool) Handle(_ context.Context, o normalizer.Observation) []types.Op {
	if o.Metric == nil || o.MetricQuery != "sentinel_rpc_mempool_txs" {
		return nil
	}
	val := o.Metric.Labels["validator"]
	// +4ns offset avoids PK collision with sibling handlers.
	sv := types.SampleValidator{
		ClusterID: h.cluster, Validator: val, Time: o.Metric.Time.Add(4 * time.Microsecond), Tier: 0,
		MempoolTxs: int32(o.Metric.Value),
	}
	return []types.Op{{Kind: types.OpUpsertSampleValidator, SampleValid: &sv}}
}

func init() {
	handlers.Register("validator.mempool",
		func(cluster string) handlers.Handler { return NewMempool(cluster) },
		validatorMempoolDoc)
}
