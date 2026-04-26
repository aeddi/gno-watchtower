package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestBackfillCLIPollsToCompletion(t *testing.T) {
	var pollCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "j-1", "status": "pending"})
		case http.MethodGet:
			n := atomic.AddInt32(&pollCount, 1)
			status := "running"
			if n >= 2 {
				status = "completed"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "j-1", "status": status})
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := backfillCmd([]string{
		"--server", srv.URL,
		"--from", "2026-04-01T00:00:00Z",
		"--to", "2026-04-02T00:00:00Z",
		"--poll-interval", "10ms",
	}, &out)
	if err != nil {
		t.Fatalf("backfillCmd: %v", err)
	}
	if !strings.Contains(out.String(), "completed") {
		t.Errorf("missing completed marker:\n%s", out.String())
	}
}

func TestBackfillCLIReturnsErrorOnFailedJob(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "j-1", "status": "pending"})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "j-1", "status": "failed", "last_error": "boom"})
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := backfillCmd([]string{
		"--server", srv.URL,
		"--from", "2026-04-01T00:00:00Z",
		"--to", "2026-04-02T00:00:00Z",
		"--poll-interval", "10ms",
	}, &out)
	if err == nil {
		t.Error("expected error when job fails")
	}
}
