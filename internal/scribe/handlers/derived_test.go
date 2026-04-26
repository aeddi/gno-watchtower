package handlers

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/eventid"
	"github.com/aeddi/gno-watchtower/internal/scribe/store"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
	"github.com/aeddi/gno-watchtower/internal/scribe/writer"
)

func TestVoteMissedEmitsForAbsentVoter(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "scribe.duckdb"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	w := writer.New(s, writer.Config{BatchSize: 1, BatchWindow: 20 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	// Seed: vote_cast from node-1, but NOT node-2.
	t1 := time.Now().UTC().Add(-5 * time.Minute)
	cast := types.Event{
		EventID:   eventid.Derive(t1, "validator.vote_cast", "node-1", []byte(`{"height":100,"round":0,"vote_type":"precommit"}`)),
		ClusterID: "c1", Time: t1, IngestTime: t1, Kind: "validator.vote_cast", Subject: "node-1",
		Payload:    map[string]any{"height": int64(100), "round": int32(0), "vote_type": "precommit"},
		Provenance: types.Provenance{Type: types.ProvenanceLog},
	}
	if err := s.WriteBatch(ctx, store.Batch{Events: []types.Event{cast}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	vm := NewVoteMissed("c1", []string{"node-1", "node-2"}, s, w)
	go vm.Run(ctx)
	// Allow goroutine to subscribe before we publish.
	time.Sleep(50 * time.Millisecond)

	// Publish chain.block_committed at H=100 via writer (so it goes through Subscribe fan-out).
	committed := types.Event{
		EventID:   eventid.Derive(t1.Add(time.Second), "chain.block_committed", "_chain", []byte(`{"height":100}`)),
		ClusterID: "c1", Time: t1.Add(time.Second), IngestTime: t1.Add(time.Second),
		Kind: "chain.block_committed", Subject: "_chain",
		Payload:    map[string]any{"height": int64(100), "round": int32(0)},
		Provenance: types.Provenance{Type: types.ProvenanceLog},
	}
	w.Submit(types.Op{Kind: types.OpInsertEvent, Event: &committed})

	// Wait for VoteMissed to react and write its derived event.
	deadline := time.Now().Add(2 * time.Second)
	var found bool
	for time.Now().Before(deadline) {
		evs, _, _ := s.QueryEvents(ctx, store.EventQuery{ClusterID: "c1", Kind: "validator.vote_missed", Limit: 10})
		for _, e := range evs {
			if e.Subject == "node-2" {
				found = true
			}
		}
		if found {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !found {
		t.Fatal("validator.vote_missed for node-2 not emitted")
	}
}
