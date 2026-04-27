package kinds_test

import (
	"context"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/handlers/kinds"
	"github.com/aeddi/gno-watchtower/internal/scribe/normalizer"
	"github.com/aeddi/gno-watchtower/internal/scribe/sources/vm"
)

func TestVotingPowerUpsertsSample(t *testing.T) {
	h := kinds.NewVotingPower("c1")
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
