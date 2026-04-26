package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/normalizer"
	"github.com/aeddi/gno-watchtower/internal/scribe/sources/vm"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

func TestHeightAdvancedEmitsOnChange(t *testing.T) {
	h := NewHeight("c1")
	now := time.Now().UTC()

	emit := func(v float64) []types.Op {
		return h.Handle(context.Background(), normalizer.Observation{
			Lane:        normalizer.LaneFast,
			IngestTime:  now,
			Metric:      &vm.Sample{Labels: map[string]string{"validator": "node-1"}, Time: now, Value: v},
			MetricQuery: "sentinel_rpc_latest_block_height",
		})
	}

	if ops := emit(100); len(ops) != 1 {
		t.Fatalf("first observation must seed without event; got %d ops", len(ops))
	}
	ops := emit(101)
	if len(ops) != 2 { // 1 event + 1 sample upsert
		t.Fatalf("expected 2 ops, got %d", len(ops))
	}
	var sawEvent bool
	for _, op := range ops {
		if op.Kind == types.OpInsertEvent && op.Event.Kind == "validator.height_advanced" {
			sawEvent = true
			if op.Event.Payload["from"].(int64) != 100 || op.Event.Payload["to"].(int64) != 101 {
				t.Errorf("payload mismatch: %+v", op.Event.Payload)
			}
		}
	}
	if !sawEvent {
		t.Error("missing height_advanced event")
	}

	if ops := emit(101); len(ops) != 1 { // unchanged: only sample upsert (delta filter is in writer/store)
		t.Errorf("expected 1 op for unchanged height, got %d", len(ops))
	}
}

func TestOnlineEmitsOfflineThenOnline(t *testing.T) {
	h := NewOnline("c1")
	now := time.Now().UTC()

	emit := func(v float64, t time.Time) []types.Op {
		return h.Handle(context.Background(), normalizer.Observation{
			Lane:        normalizer.LaneFast,
			IngestTime:  t,
			Metric:      &vm.Sample{Labels: map[string]string{"validator": "node-1"}, Time: t, Value: v},
			MetricQuery: "sentinel_validator_online",
		})
	}

	if ops := emit(1, now); len(ops) != 0 {
		t.Fatalf("first observation must seed without event; got %d ops", len(ops))
	}
	// 1 -> 0 transition: went_offline.
	ops := emit(0, now.Add(time.Second))
	if len(ops) != 1 || ops[0].Event.Kind != "validator.went_offline" {
		t.Fatalf("expected went_offline, got %+v", ops)
	}
	// 0 -> 1 transition: came_online.
	ops = emit(1, now.Add(2*time.Second))
	if len(ops) != 1 || ops[0].Event.Kind != "validator.came_online" {
		t.Fatalf("expected came_online, got %+v", ops)
	}
}

func TestPeersUpsertsSampleByMetricName(t *testing.T) {
	h := NewPeers("c1")
	now := time.Now().UTC()
	in := h.Handle(context.Background(), normalizer.Observation{
		Lane:        normalizer.LaneFast,
		IngestTime:  now,
		Metric:      &vm.Sample{Labels: map[string]string{"validator": "node-1"}, Time: now, Value: 3},
		MetricQuery: "inbound_peers_gauge",
	})
	if len(in) != 1 || in[0].SampleValid.PeerCountIn != 3 {
		t.Fatalf("inbound: got %+v", in)
	}
	out := h.Handle(context.Background(), normalizer.Observation{
		Lane:        normalizer.LaneFast,
		IngestTime:  now,
		Metric:      &vm.Sample{Labels: map[string]string{"validator": "node-1"}, Time: now, Value: 5},
		MetricQuery: "outbound_peers_gauge",
	})
	if len(out) != 1 || out[0].SampleValid.PeerCountOut != 5 {
		t.Fatalf("outbound: got %+v", out)
	}
}

func TestMempoolUpsertsSample(t *testing.T) {
	h := NewMempool("c1")
	now := time.Now().UTC()
	ops := h.Handle(context.Background(), normalizer.Observation{
		Lane:        normalizer.LaneFast,
		IngestTime:  now,
		Metric:      &vm.Sample{Labels: map[string]string{"validator": "node-1"}, Time: now, Value: 5},
		MetricQuery: "sentinel_rpc_mempool_txs",
	})
	if len(ops) != 1 || ops[0].SampleValid.MempoolTxs != 5 {
		t.Errorf("got %+v", ops)
	}
}

func TestVotingPowerUpsertsSample(t *testing.T) {
	h := NewVotingPower("c1")
	now := time.Now().UTC()
	ops := h.Handle(context.Background(), normalizer.Observation{
		Lane:        normalizer.LaneFast,
		IngestTime:  now,
		Metric:      &vm.Sample{Labels: map[string]string{"validator": "node-1"}, Time: now, Value: 100},
		MetricQuery: "sentinel_rpc_validator_voting_power",
	})
	if len(ops) != 1 || ops[0].SampleValid.VotingPower != 100 {
		t.Errorf("got %+v", ops)
	}
}

func TestValsetSizeAggregatesChainSample(t *testing.T) {
	h := NewValsetSize("c1")
	now := time.Now().UTC()

	// First batch: feed two member observations at the same time.
	for _, vp := range []float64{100, 200} {
		_ = h.Handle(context.Background(), normalizer.Observation{
			Lane:        normalizer.LaneFast,
			IngestTime:  now,
			Metric:      &vm.Sample{Labels: map[string]string{"address": "g1abc"}, Time: now, Value: vp},
			MetricQuery: "sentinel_rpc_validator_set_power",
		})
	}
	// Aggregate is computed per poll-tick. ValsetSize aggregates by Metric.Time;
	// the third call with a NEW time emits the chain sample for the previous tick.
	later := now.Add(time.Second)
	ops := h.Handle(context.Background(), normalizer.Observation{
		Lane:        normalizer.LaneFast,
		IngestTime:  later,
		Metric:      &vm.Sample{Labels: map[string]string{"address": "g1xyz"}, Time: later, Value: 50},
		MetricQuery: "sentinel_rpc_validator_set_power",
	})
	// First poll's aggregate: 2 members, total VP = 300. Should land in samples_chain.
	if len(ops) == 0 {
		t.Fatal("no chain sample emitted at tick rollover")
	}
	var sawChain bool
	for _, op := range ops {
		if op.Kind == types.OpUpsertSampleChain && op.SampleChain.ValsetSize == 2 && op.SampleChain.TotalVotingPower == 300 {
			sawChain = true
		}
	}
	if !sawChain {
		t.Errorf("expected chain sample with size=2 totalVP=300, got %+v", ops)
	}
}
