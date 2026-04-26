package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPostBackfillCreatesJob(t *testing.T) {
	srv, _ := newTestServer(t)
	body, _ := json.Marshal(map[string]any{
		"from": time.Now().Add(-2 * time.Hour),
		"to":   time.Now().Add(-1 * time.Hour),
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/backfill", bytes.NewReader(body))
	srv.http().ServeHTTP(rr, req)
	if rr.Code != 202 {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.ID == "" {
		t.Error("missing id")
	}
}

func TestGetBackfillReturns404OnUnknown(t *testing.T) {
	srv, _ := newTestServer(t)
	rr := httptest.NewRecorder()
	srv.http().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/backfill/missing", nil))
	if rr.Code != 404 {
		t.Errorf("status = %d", rr.Code)
	}
}
