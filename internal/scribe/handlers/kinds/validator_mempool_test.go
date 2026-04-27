package kinds_test

import (
	"context"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/handlers/kinds"
	"github.com/aeddi/gno-watchtower/internal/scribe/normalizer"
	"github.com/aeddi/gno-watchtower/internal/scribe/sources/vm"
)

func TestMempoolUpsertsSample(t *testing.T) {
	h := kinds.NewMempool("c1")
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
