package kinds

import (
	"context"
	_ "embed"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/handlers"
	"github.com/aeddi/gno-watchtower/internal/scribe/normalizer"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

//go:embed validator_peers.md
var validatorPeersDoc string

// Peers watches sentinel_rpc_peers and upserts peer_count_in / peer_count_out
// samples depending on the direction label.
type Peers struct {
	cluster string
}

func NewPeers(cluster string) *Peers { return &Peers{cluster: cluster} }

func (Peers) Name() string { return "peers" }

// Meta returns the descriptor used by the handler registry and /api/handlers.
// Peers only upserts samples_validator (peer_count_in / peer_count_out); it
// does not emit events. Kind is a descriptive label used by the registry.
func (Peers) Meta() handlers.Meta {
	return handlers.Meta{
		Kind:        "validator.peers",
		Source:      handlers.SourceMetric,
		Description: "Inbound and outbound peer counts per validator (sample upsert only, no event).",
		DocRef:      "/docs/handlers/validator.peers",
	}
}

func (h *Peers) Handle(_ context.Context, o normalizer.Observation) []types.Op {
	// gnoland exports peer counts via OTLP as inbound_peers_gauge and
	// outbound_peers_gauge (no `direction` label — the direction is in the
	// metric name). Each metric carries `{validator}`.
	if o.Metric == nil {
		return nil
	}
	val := o.Metric.Labels["validator"]
	if val == "" {
		return nil
	}
	// +2ns / +3ns offsets avoid PK collision with sibling handlers writing the
	// same (cluster, validator) at the same metric time.
	offset := 2 * time.Microsecond
	if o.MetricQuery == "outbound_peers_gauge" {
		offset = 3 * time.Microsecond
	}
	sv := types.SampleValidator{ClusterID: h.cluster, Validator: val, Time: o.Metric.Time.Add(offset), Tier: 0}
	switch o.MetricQuery {
	case "inbound_peers_gauge":
		sv.PeerCountIn = int16(o.Metric.Value)
	case "outbound_peers_gauge":
		sv.PeerCountOut = int16(o.Metric.Value)
	default:
		return nil
	}
	return []types.Op{{Kind: types.OpUpsertSampleValidator, SampleValid: &sv}}
}

func init() {
	handlers.Register("validator.peers",
		func(cluster string) handlers.Handler { return NewPeers(cluster) },
		validatorPeersDoc)
}
