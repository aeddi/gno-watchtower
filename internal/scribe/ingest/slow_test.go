package ingest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/normalizer"
	"github.com/aeddi/gno-watchtower/internal/scribe/sources/vm"
)

func TestSlowLanePushesObservations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[
            {"metric":{"validator":"node-1"},"value":[1714039200,"100"]}]}}`))
	}))
	defer srv.Close()

	out := make(chan normalizer.Observation, 8)
	lane := NewSlowLane(vm.New(srv.URL), []string{"avg(build_block_hist)"}, 50*time.Millisecond, out)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go lane.Run(ctx)

	select {
	case o := <-out:
		if o.Lane != normalizer.LaneSlow {
			t.Errorf("lane = %q, want %q", o.Lane, normalizer.LaneSlow)
		}
		if o.Metric == nil || o.Metric.Labels["validator"] != "node-1" {
			t.Errorf("got %+v", o)
		}
	case <-time.After(time.Second):
		t.Fatal("no observation produced")
	}
}
