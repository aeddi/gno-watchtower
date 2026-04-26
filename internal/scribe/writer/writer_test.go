package writer

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/store"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

func TestWriterBatchesAndCommits(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "scribe.duckdb"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	w := New(s, Config{BatchSize: 2, BatchWindow: 100 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); _ = w.Run(ctx) }()

	now := time.Now().UTC().Truncate(time.Millisecond)
	for i := 0; i < 5; i++ {
		ev := types.Event{
			EventID:   string(rune(0x30+i)) + "1JCT0AAA0AAA0AAA0AAA0AAA00",
			ClusterID: "c1", Time: now, IngestTime: now,
			Kind: "x", Subject: "node-1",
			Payload: map[string]any{"i": float64(i)}, Provenance: types.Provenance{Type: types.ProvenanceMetric},
		}
		// Pad / clamp to 26-char ULID.
		ev.EventID = (ev.EventID + "0000000000000000")[:26]
		w.Submit(types.Op{Kind: types.OpInsertEvent, Event: &ev})
	}

	time.Sleep(300 * time.Millisecond)
	cancel()
	wg.Wait()

	out, _, err := s.QueryEvents(context.Background(), store.EventQuery{ClusterID: "c1", Limit: 10})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(out) < 5 {
		t.Errorf("expected ≥5 events, got %d", len(out))
	}
}

func TestWriterFanOutBroadcastsEvents(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "scribe.duckdb"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	w := New(s, Config{BatchSize: 1, BatchWindow: 50 * time.Millisecond})
	sub := w.Subscribe(4)
	defer w.Unsubscribe(sub)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = w.Run(ctx) }()

	now := time.Now().UTC().Truncate(time.Millisecond)
	ev := types.Event{
		EventID: "01JCT0BBB0BBB0BBB0BBB0BBB0", ClusterID: "c1", Time: now, IngestTime: now,
		Kind: "x", Subject: "node-1",
		Payload: map[string]any{}, Provenance: types.Provenance{Type: types.ProvenanceMetric},
	}
	w.Submit(types.Op{Kind: types.OpInsertEvent, Event: &ev})

	select {
	case got := <-sub:
		if got.EventID != ev.EventID {
			t.Errorf("event_id mismatch")
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber received nothing")
	}
}
