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

func TestStateBatchReturnsAllSubjects(t *testing.T) {
	srv, s := newTestServer(t)
	now := time.Now().UTC().Truncate(time.Second)
	for _, sub := range []string{"node-1", "node-2"} {
		a := types.Anchor{
			ClusterID: "c1", Subject: sub, Time: now,
			FullState:     map[string]any{"config_hash": "h-" + sub},
			EventsThrough: "",
		}
		_ = s.WriteBatch(context.Background(), store.Batch{Anchors: []types.Anchor{a}})
	}

	rr := httptest.NewRecorder()
	srv.http().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/state?subjects=node-1,node-2", nil))
	if rr.Code != 200 {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		States map[string]map[string]any `json:"states"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if len(resp.States) != 2 {
		t.Fatalf("expected 2 states, got %d", len(resp.States))
	}
}
