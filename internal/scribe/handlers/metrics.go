package handlers

import (
	"context"
	"encoding/json"
	"sync"
	"time"

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
	// +1ns offset avoids PK collision when sibling metric handlers (Mempool,
	// VotingPower, Peers) write at the same VM-reported metric time.
	sv := types.SampleValidator{
		ClusterID: h.cluster, Validator: val, Time: o.Metric.Time.Add(1 * time.Microsecond), Tier: 0,
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

func (Online) Name() string { return "online" }

func (h *Online) Handle(_ context.Context, o normalizer.Observation) []types.Op {
	if o.Metric == nil || o.MetricQuery != "sentinel_validator_online" {
		return nil
	}
	val := o.Metric.Labels["validator"]
	online := o.Metric.Value > 0

	h.mu.Lock()
	prev, had := h.last[val]
	prevSeen := h.lastSeen[val]
	h.last[val] = online
	if online {
		h.lastSeen[val] = o.Metric.Time
	}
	h.mu.Unlock()

	if !had || prev == online {
		return nil
	}
	if online {
		gap := o.Metric.Time.Sub(prevSeen).String()
		payload := kinds.ValidatorCameOnline{GapDuration: gap}
		pb, _ := json.Marshal(payload)
		ev := types.Event{
			EventID:   eventid.Derive(o.Metric.Time, payload.Kind(), val, pb),
			ClusterID: h.cluster, Time: o.Metric.Time, IngestTime: o.IngestTime,
			Kind: payload.Kind(), Subject: val,
			Payload: map[string]any{"gap_duration": gap},
			Provenance: types.Provenance{
				Type: types.ProvenanceMetric, Query: o.MetricQuery,
				Metric: &types.MetricRef{Backend: "vm", Query: o.MetricQuery, Value: o.Metric.Value, At: o.Metric.Time},
			},
		}
		return []types.Op{{Kind: types.OpInsertEvent, Event: &ev}}
	}
	payload := kinds.ValidatorWentOffline{LastSeen: prevSeen}
	pb, _ := json.Marshal(payload)
	ev := types.Event{
		EventID:   eventid.Derive(o.Metric.Time, payload.Kind(), val, pb),
		ClusterID: h.cluster, Time: o.Metric.Time, IngestTime: o.IngestTime,
		Kind: payload.Kind(), Subject: val,
		Payload: map[string]any{"last_seen": prevSeen},
		Provenance: types.Provenance{
			Type: types.ProvenanceMetric, Query: o.MetricQuery,
			Metric: &types.MetricRef{Backend: "vm", Query: o.MetricQuery, Value: o.Metric.Value, At: o.Metric.Time},
		},
	}
	return []types.Op{{Kind: types.OpInsertEvent, Event: &ev}}
}

// Peers watches sentinel_rpc_peers and upserts peer_count_in / peer_count_out
// samples depending on the direction label.
type Peers struct {
	cluster string
}

func NewPeers(cluster string) *Peers { return &Peers{cluster: cluster} }

func (Peers) Name() string { return "peers" }

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

// Mempool watches sentinel_rpc_mempool_txs and upserts mempool_txs samples.
type Mempool struct {
	cluster string
}

func NewMempool(cluster string) *Mempool { return &Mempool{cluster: cluster} }

func (Mempool) Name() string { return "mempool" }

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

// VotingPower watches sentinel_rpc_validator_voting_power and upserts voting_power samples.
type VotingPower struct {
	cluster string
}

func NewVotingPower(cluster string) *VotingPower { return &VotingPower{cluster: cluster} }

func (VotingPower) Name() string { return "voting_power" }

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

func (ValsetSize) Name() string { return "valset_size" }

func (h *ValsetSize) Handle(_ context.Context, o normalizer.Observation) []types.Op {
	if o.Metric == nil || o.MetricQuery != "sentinel_rpc_validator_set_power" {
		return nil
	}
	t := o.Metric.Time

	h.mu.Lock()
	defer h.mu.Unlock()

	// Flush any prior ticks (Time < t) — they're complete now.
	var ops []types.Op
	for past, acc := range h.pending {
		if past.Before(t) {
			ops = append(ops, types.Op{Kind: types.OpUpsertSampleChain, SampleChain: &types.SampleChain{
				ClusterID:        h.cluster,
				Time:             past,
				Tier:             0,
				ValsetSize:       int16(acc.count),
				TotalVotingPower: acc.total,
			}})
			delete(h.pending, past)
		}
	}

	// Accumulate the current observation.
	acc, ok := h.pending[t]
	if !ok {
		acc = &valsetAccum{}
		h.pending[t] = acc
	}
	acc.count++
	acc.total += int64(o.Metric.Value)
	return ops
}
