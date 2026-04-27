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

func TestValsetSizeAggregatesChainSample(t *testing.T) {
	h := kinds.NewValsetSize("c1")
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
