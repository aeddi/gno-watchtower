package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/store"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

func TestSamplesEndpointBuckets(t *testing.T) {
	srv, s := newTestServer(t)
	base := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Minute)
	for i := 0; i < 6; i++ {
		sv := types.SampleValidator{
			ClusterID: "c1", Validator: "node-1",
			Time: base.Add(time.Duration(i) * time.Minute), Tier: 0,
			Height: int64(100 + i), CPUPct: float32(10 + i),
			LastObserved: base.Add(time.Duration(i) * time.Minute),
		}
		_ = s.WriteBatch(context.Background(), store.Batch{SamplesValidator: []types.SampleValidator{sv}})
	}

	url := "/api/samples?subject=node-1&from=" + base.Format(time.RFC3339Nano) +
		"&to=" + base.Add(10*time.Minute).Format(time.RFC3339Nano) + "&step=1m"
	rr := httptest.NewRecorder()
	srv.http().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, url, nil))
	if rr.Code != 200 {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Buckets []map[string]any `json:"buckets"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if len(resp.Buckets) < 6 {
		t.Errorf("got %d buckets", len(resp.Buckets))
	}
}

func TestSamplesEndpointRejectsTooManyBuckets(t *testing.T) {
	srv, _ := newTestServer(t)
	url := "/api/samples?subject=node-1&from=2026-01-01T00:00:00Z&to=2026-12-31T00:00:00Z&step=1s"
	rr := httptest.NewRecorder()
	srv.http().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, url, nil))
	if rr.Code != 400 {
		t.Errorf("status = %d (want 400)", rr.Code)
	}
}
