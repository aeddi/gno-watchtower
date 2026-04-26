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
