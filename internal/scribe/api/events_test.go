package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/eventid"
	"github.com/aeddi/gno-watchtower/internal/scribe/store"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

func newTestServer(t *testing.T) (*Server, store.Store) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "scribe.duckdb"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return New(Deps{Store: s, ClusterID: "c1"}), s
}

func TestGetEventsReturnsSeeded(t *testing.T) {
	srv, s := newTestServer(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	ev := types.Event{
		EventID:   eventid.Derive(now, "x", "node-1", []byte("{}")),
		ClusterID: "c1", Time: now, IngestTime: now, Kind: "x", Subject: "node-1",
		Payload: map[string]any{}, Provenance: types.Provenance{Type: types.ProvenanceMetric},
	}
	if err := s.WriteBatch(context.Background(), store.Batch{Events: []types.Event{ev}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rr := httptest.NewRecorder()
	srv.http().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/events?subject=node-1&limit=10", nil))
	if rr.Code != 200 {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Events []map[string]any `json:"events"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if len(resp.Events) != 1 {
		t.Fatalf("got %d events", len(resp.Events))
	}
}

func TestEventsFilterBySeverityAndState(t *testing.T) {
	srv, st := newTestServer(t)
	ctx := context.Background()
	now := time.Now().UTC()

	st.WriteBatch(ctx, store.Batch{Events: []types.Event{
		{
			EventID: "01J0", ClusterID: "c1", Time: now, IngestTime: now,
			Kind: "diagnostic.bft_at_risk_v1", Subject: types.SubjectChain,
			Severity: "error", State: "open",
			Payload: map[string]any{}, Provenance: types.Provenance{Type: types.ProvenanceRule},
		},
		{
			EventID: "01J1", ClusterID: "c1", Time: now, IngestTime: now,
			Kind: "diagnostic.block_missed_v1", Subject: "moul-3",
			Severity: "warning", State: "open",
			Payload: map[string]any{}, Provenance: types.Provenance{Type: types.ProvenanceRule},
		},
		{
			EventID: "01J2", ClusterID: "c1", Time: now, IngestTime: now,
			Kind: "validator.vote_cast", Subject: "moul-3",
			Payload: map[string]any{}, Provenance: types.Provenance{Type: types.ProvenanceLog},
		},
	}})

	r := httptest.NewRequest(http.MethodGet, "/api/events?severity=error", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("severity=error: %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "01J0") || strings.Contains(body, "01J1") || strings.Contains(body, "01J2") {
		t.Errorf("severity=error returned wrong rows: %s", body)
	}

	r = httptest.NewRequest(http.MethodGet, "/api/events?severity=warning,error&state=open", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("severity list + state=open: %d %s", w.Code, w.Body.String())
	}
	body = w.Body.String()
	if !strings.Contains(body, "01J0") || !strings.Contains(body, "01J1") || strings.Contains(body, "01J2") {
		t.Errorf("severity list + state=open returned wrong rows: %s", body)
	}
}
