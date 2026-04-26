package backfill

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/normalizer"
	"github.com/aeddi/gno-watchtower/internal/scribe/sources/loki"
	"github.com/aeddi/gno-watchtower/internal/scribe/sources/vm"
	"github.com/aeddi/gno-watchtower/internal/scribe/store"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

func TestEngineRunsCompleteJob(t *testing.T) {
	vmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
	}))
	defer vmSrv.Close()
	lokiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"streams","result":[]}}`))
	}))
	defer lokiSrv.Close()

	s, _ := store.Open(filepath.Join(t.TempDir(), "scribe.duckdb"))
	defer s.Close()

	out := make(chan types.Op, 16)
	n := normalizer.New(out, nil)

	e := New(Deps{
		Store:       s,
		ClusterID:   "c1",
		VM:          vm.New(vmSrv.URL),
		Loki:        loki.New(lokiSrv.URL),
		FastQueries: []string{"sentinel_validator_online"},
		LogStreams:  []string{`{validator=~".+"}`},
		ChunkSize:   time.Hour,
		Normalizer:  n,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go n.Run(ctx)

	job := store.BackfillJob{
		ID: "j1", ClusterID: "c1",
		From: time.Now().Add(-3 * time.Hour), To: time.Now().Add(-2 * time.Hour),
		ChunkSize: time.Hour, StartedAt: time.Now(), LastProgressAt: time.Now(),
		Status: "running",
	}
	if err := s.UpsertBackfillJob(ctx, job); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if err := e.Run(ctx, job); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, _ := s.GetBackfillJob(ctx, "j1")
	if got == nil || got.Status != "completed" {
		t.Fatalf("status = %v", got)
	}
}
