package projection

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/eventid"
	"github.com/aeddi/gno-watchtower/internal/scribe/store"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

func TestProjectAppliesEventsAfterAnchor(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "scribe.duckdb"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	ctx := context.Background()
	t0 := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)

	// Anchor at t0 with empty peers.
	a := types.Anchor{
		ClusterID: "c1", Subject: "node-1", Time: t0,
		FullState:     map[string]any{"peers": map[string]any{}, "config_hash": "h0"},
		EventsThrough: "",
	}
	if err := s.WriteBatch(ctx, store.Batch{Anchors: []types.Anchor{a}}); err != nil {
		t.Fatalf("anchor: %v", err)
	}

	// peer_connected event 30 min later.
	t1 := t0.Add(30 * time.Minute)
	payload := []byte(`{"peer":"node-2","peer_id":"abc","direction":"out"}`)
	ev := types.Event{
		EventID:   eventid.Derive(t1, "validator.peer_connected", "node-1", payload),
		ClusterID: "c1", Time: t1, IngestTime: t1,
		Kind: "validator.peer_connected", Subject: "node-1",
		Payload:    map[string]any{"peer": "node-2", "peer_id": "abc", "direction": "out"},
		Provenance: types.Provenance{Type: types.ProvenanceLog},
	}
	if err := s.WriteBatch(ctx, store.Batch{Events: []types.Event{ev}}); err != nil {
		t.Fatalf("event: %v", err)
	}

	st, replayed, err := ProjectStateAt(ctx, s, "c1", "node-1", t1.Add(time.Minute))
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if replayed != 1 {
		t.Errorf("replayed=%d want 1", replayed)
	}
	peers, _ := st["peers"].(map[string]any)
	if peers == nil || peers["abc"] == nil {
		t.Errorf("peer not applied: %+v", st)
	}
}
