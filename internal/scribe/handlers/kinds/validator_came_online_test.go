package kinds_test

import (
	"context"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/handlers/kinds"
	"github.com/aeddi/gno-watchtower/internal/scribe/normalizer"
	"github.com/aeddi/gno-watchtower/internal/scribe/sources/vm"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

func TestOnlineEmitsOfflineThenOnline(t *testing.T) {
	h := kinds.NewOnline("c1")
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
