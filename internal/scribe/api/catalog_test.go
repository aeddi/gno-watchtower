package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEventKindsCatalog(t *testing.T) {
	srv, _ := newTestServer(t)
	rr := httptest.NewRecorder()
	srv.http().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/event-kinds", nil))
	if rr.Code != 200 {
		t.Fatalf("status = %d", rr.Code)
	}
	var resp struct {
		Kinds []struct {
			Kind   string         `json:"kind"`
			Schema map[string]any `json:"schema"`
		} `json:"kinds"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if len(resp.Kinds) < 15 {
		t.Errorf("got %d kinds, want ≥15", len(resp.Kinds))
	}
}
