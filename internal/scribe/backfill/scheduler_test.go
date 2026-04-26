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

func TestSchedulerRunsPendingJobs(t *testing.T) {
	vmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
	}))
	defer vmSrv.Close()
	lokiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"streams","result":[]}}`))
	}))
	defer lokiSrv.Close()

	s, _ := store.Open(filepath.Join(t.TempDir(), "scribe.duckdb"))
	defer s.Close()

	out := make(chan types.Op, 8)
	n := normalizer.New(out, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go n.Run(ctx)

	engine := New(Deps{
		Store: s, ClusterID: "c1",
		VM: vm.New(vmSrv.URL), Loki: loki.New(lokiSrv.URL),
		FastQueries: []string{"x"}, LogStreams: []string{`{x="y"}`},
		ChunkSize: time.Hour, Normalizer: n,
	})
	sched := NewScheduler(s, engine, "c1", SchedulerConfig{
		PollInterval:     50 * time.Millisecond,
		ResumeStaleAfter: 5 * time.Minute,
	})
	go sched.Run(ctx)

	// Seed a pending job; the scheduler should pick it up.
	job := store.BackfillJob{
		ID: "j-pending", ClusterID: "c1",
		From: time.Now().Add(-2 * time.Hour), To: time.Now().Add(-1 * time.Hour),
		ChunkSize: time.Hour, StartedAt: time.Now(), LastProgressAt: time.Now(),
		Status: "pending",
	}
	if err := s.UpsertBackfillJob(ctx, job); err != nil {
		t.Fatalf("seed: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, _ := s.GetBackfillJob(ctx, "j-pending")
		if got != nil && got.Status == "completed" {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("scheduler did not complete the pending job within 3s")
}
