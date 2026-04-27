package kinds

import (
	"context"
	_ "embed"
	"encoding/json"
	"sync"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/eventid"
	"github.com/aeddi/gno-watchtower/internal/scribe/handlers"
	sk "github.com/aeddi/gno-watchtower/internal/scribe/kinds"
	"github.com/aeddi/gno-watchtower/internal/scribe/normalizer"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

//go:embed validator_came_online.md
var validatorCameOnlineDoc string

// Online watches sentinel_validator_online and emits:
//   - validator.went_offline when the value transitions 1→0
//   - validator.came_online when the value transitions 0→1
type Online struct {
	cluster  string
	mu       sync.Mutex
	last     map[string]bool
	lastSeen map[string]time.Time
}

func NewOnline(cluster string) *Online {
	return &Online{cluster: cluster, last: map[string]bool{}, lastSeen: map[string]time.Time{}}
}

func (o *Online) Name() string { return "online" }

// Meta returns the descriptor used by the handler registry and /api/handlers.
// Online emits two kinds conditionally: validator.came_online (0→1) and
// validator.went_offline (1→0). The primary kind listed here is came_online.
func (o *Online) Meta() handlers.Meta {
	return handlers.Meta{
		Kind:        "validator.came_online",
		Source:      handlers.SourceMetric,
		Description: "Validator online/offline state transitions; emits came_online or went_offline.",
		DocRef:      "/docs/handlers/validator.came_online",
	}
}

func (o *Online) Handle(_ context.Context, obs normalizer.Observation) []types.Op {
	if obs.Metric == nil || obs.MetricQuery != "sentinel_validator_online" {
		return nil
	}
	val := obs.Metric.Labels["validator"]
	online := obs.Metric.Value > 0

	o.mu.Lock()
	prev, had := o.last[val]
	prevSeen := o.lastSeen[val]
	o.last[val] = online
	if online {
		o.lastSeen[val] = obs.Metric.Time
	}
	o.mu.Unlock()

	if !had || prev == online {
		return nil
	}
	if online {
		gap := obs.Metric.Time.Sub(prevSeen).String()
		payload := sk.ValidatorCameOnline{GapDuration: gap}
		pb, _ := json.Marshal(payload)
		ev := types.Event{
			EventID:   eventid.Derive(obs.Metric.Time, payload.Kind(), val, pb),
			ClusterID: o.cluster, Time: obs.Metric.Time, IngestTime: obs.IngestTime,
			Kind: payload.Kind(), Subject: val,
			Payload: map[string]any{"gap_duration": gap},
			Provenance: types.Provenance{
				Type: types.ProvenanceMetric, Query: obs.MetricQuery,
				Metric: &types.MetricRef{Backend: "vm", Query: obs.MetricQuery, Value: obs.Metric.Value, At: obs.Metric.Time},
			},
		}
		return []types.Op{{Kind: types.OpInsertEvent, Event: &ev}}
	}
	payload := sk.ValidatorWentOffline{LastSeen: prevSeen}
	pb, _ := json.Marshal(payload)
	ev := types.Event{
		EventID:   eventid.Derive(obs.Metric.Time, payload.Kind(), val, pb),
		ClusterID: o.cluster, Time: obs.Metric.Time, IngestTime: obs.IngestTime,
		Kind: payload.Kind(), Subject: val,
		Payload: map[string]any{"last_seen": prevSeen},
		Provenance: types.Provenance{
			Type: types.ProvenanceMetric, Query: obs.MetricQuery,
			Metric: &types.MetricRef{Backend: "vm", Query: obs.MetricQuery, Value: obs.Metric.Value, At: obs.Metric.Time},
		},
	}
	return []types.Op{{Kind: types.OpInsertEvent, Event: &ev}}
}

func init() {
	handlers.Register("validator.came_online",
		func(cluster string) handlers.Handler { return NewOnline(cluster) },
		validatorCameOnlineDoc)
}
