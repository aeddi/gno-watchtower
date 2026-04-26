package handlers

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/aeddi/gno-watchtower/internal/scribe/eventid"
	"github.com/aeddi/gno-watchtower/internal/scribe/kinds"
	"github.com/aeddi/gno-watchtower/internal/scribe/normalizer"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

// Height watches sentinel_rpc_latest_block_height and emits:
//   - validator.height_advanced event when the per-validator value increases
//   - samples_validator upsert with height + last_observed every poll
type Height struct {
	cluster string
	mu      sync.Mutex
	last    map[string]int64 // validator -> last seen height
}

func NewHeight(cluster string) *Height {
	return &Height{cluster: cluster, last: map[string]int64{}}
}

func (h *Height) Name() string { return "height" }

func (h *Height) Handle(_ context.Context, o normalizer.Observation) []types.Op {
	if o.Metric == nil || o.MetricQuery != "sentinel_rpc_latest_block_height" {
		return nil
	}
	val := o.Metric.Labels["validator"]
	if val == "" {
		return nil
	}
	height := int64(o.Metric.Value)

	h.mu.Lock()
	prev, hadPrev := h.last[val]
	h.last[val] = height
	h.mu.Unlock()

	ops := make([]types.Op, 0, 2)

	// Always upsert sample (delta-filter is store-side / writer-side).
	sv := types.SampleValidator{
		ClusterID: h.cluster, Validator: val, Time: o.Metric.Time, Tier: 0,
		Height: height, LastObserved: o.Metric.Time,
	}
	ops = append(ops, types.Op{Kind: types.OpUpsertSampleValidator, SampleValid: &sv})

	if !hadPrev || prev == height {
		return ops
	}

	payload := kinds.ValidatorHeightAdvanced{From: prev, To: height}
	pb, _ := json.Marshal(payload)
	ev := types.Event{
		EventID:    eventid.Derive(o.Metric.Time, payload.Kind(), val, pb),
		ClusterID:  h.cluster,
		Time:       o.Metric.Time,
		IngestTime: o.IngestTime,
		Kind:       payload.Kind(),
		Subject:    val,
		Payload:    map[string]any{"from": prev, "to": height},
		Provenance: types.Provenance{
			Type:  types.ProvenanceMetric,
			Query: o.MetricQuery,
			Metric: &types.MetricRef{
				Backend: "vm",
				Query:   o.MetricQuery,
				Value:   o.Metric.Value,
				At:      o.Metric.Time,
			},
		},
	}
	ops = append(ops, types.Op{Kind: types.OpInsertEvent, Event: &ev})
	return ops
}
