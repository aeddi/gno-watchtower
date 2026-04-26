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

func TestFastLanePushesObservations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[
            {"metric":{"validator":"node-1"},"value":[1714039200,"100"]}]}}`))
	}))
	defer srv.Close()

	out := make(chan normalizer.Observation, 8)
	lane := NewFastLane(vm.New(srv.URL), []string{"sentinel_rpc_latest_block_height"}, 50*time.Millisecond, out)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go lane.Run(ctx)

	select {
	case o := <-out:
		if o.Metric == nil || o.Metric.Labels["validator"] != "node-1" {
			t.Errorf("got %+v", o)
		}
	case <-time.After(time.Second):
		t.Fatal("no observation produced")
	}
}
