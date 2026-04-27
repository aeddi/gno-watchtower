package kinds_test

import (
	"context"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/handlers/kinds"
	"github.com/aeddi/gno-watchtower/internal/scribe/normalizer"
	"github.com/aeddi/gno-watchtower/internal/scribe/sources/vm"
)

func TestPeersUpsertsSampleByMetricName(t *testing.T) {
	h := kinds.NewPeers("c1")
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
