package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

func openTempStore(t *testing.T) *duckStore {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "scribe.duckdb"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestWriteBatchInsertsEvent(t *testing.T) {
	s := openTempStore(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	ev := types.Event{
		EventID:    "01JCT0AAA0AAA0AAA0AAA0AAA0",
		ClusterID:  "c1",
		Time:       now,
		IngestTime: now,
		Kind:       "validator.height_advanced",
		Subject:    "node-1",
		Payload:    map[string]any{"from": float64(100), "to": float64(101)},
		Provenance: types.Provenance{Type: types.ProvenanceMetric, Query: "x"},
	}
	if err := s.WriteBatch(context.Background(), Batch{Events: []types.Event{ev}}); err != nil {
		t.Fatalf("WriteBatch: %v", err)
	}
	out, _, err := s.QueryEvents(context.Background(), EventQuery{ClusterID: "c1", Limit: 10})
	if err != nil {
		t.Fatalf("QueryEvents: %v", err)
	}
	if len(out) != 1 || out[0].EventID != ev.EventID {
		t.Fatalf("got %d events, want 1 with id %q", len(out), ev.EventID)
	}
}

func TestWriteBatchEventIdempotent(t *testing.T) {
	s := openTempStore(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	ev := types.Event{
		EventID: "01JCT0AAA0AAA0AAA0AAA0AAA0", ClusterID: "c1",
		Time: now, IngestTime: now,
		Kind: "validator.height_advanced", Subject: "node-1",
		Payload: map[string]any{}, Provenance: types.Provenance{Type: types.ProvenanceMetric},
	}
	for i := 0; i < 3; i++ {
		if err := s.WriteBatch(context.Background(), Batch{Events: []types.Event{ev}}); err != nil {
			t.Fatalf("WriteBatch: %v", err)
		}
	}
	out, _, _ := s.QueryEvents(context.Background(), EventQuery{ClusterID: "c1", Limit: 10})
	if len(out) != 1 {
		t.Errorf("expected 1 row after 3 identical inserts, got %d", len(out))
	}
}

func TestWriteBatchSampleUpsert(t *testing.T) {
	s := openTempStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	sv := types.SampleValidator{
		ClusterID: "c1", Validator: "node-1", Time: now, Tier: 0,
		Height: 100, VotingPower: 10, MempoolTxs: 5, CPUPct: 12.5, MemPct: 33,
		LastObserved: now,
	}
	for i := 0; i < 3; i++ {
		if err := s.WriteBatch(context.Background(), Batch{SamplesValidator: []types.SampleValidator{sv}}); err != nil {
			t.Fatalf("WriteBatch: %v", err)
		}
	}
	got, err := s.GetLatestSampleValidator(context.Background(), "c1", "node-1", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("GetLatestSampleValidator: %v", err)
	}
	if got == nil || got.Height != 100 {
		t.Errorf("got %+v, want Height=100", got)
	}
}
