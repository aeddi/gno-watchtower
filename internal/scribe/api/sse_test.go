package api

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/eventid"
	"github.com/aeddi/gno-watchtower/internal/scribe/store"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
	"github.com/aeddi/gno-watchtower/internal/scribe/writer"
)

func TestSSEDeliversNewEvents(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "scribe.duckdb"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	w := writer.New(s, writer.Config{BatchSize: 1, BatchWindow: 20 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	srv := New(Deps{Store: s, Writer: w, ClusterID: "c1"})
	hsrv := httptest.NewServer(srv.http())
	defer hsrv.Close()

	resp, err := http.Get(hsrv.URL + "/api/events/stream")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	go func() {
		time.Sleep(100 * time.Millisecond)
		now := time.Now().UTC()
		ev := types.Event{
			EventID:   eventid.Derive(now, "x", "node-1", []byte("{}")),
			ClusterID: "c1", Time: now, IngestTime: now, Kind: "x", Subject: "node-1",
			Payload: map[string]any{}, Provenance: types.Provenance{Type: types.ProvenanceMetric},
		}
		w.Submit(types.Op{Kind: types.OpInsertEvent, Event: &ev})
	}()

	// Read in a goroutine so we can bound the wait without blocking on Scan().
	lines := make(chan string, 16)
	go func() {
		defer close(lines)
		scn := bufio.NewScanner(resp.Body)
		for scn.Scan() {
			lines <- scn.Text()
		}
	}()

	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-deadline:
			// Closing the body unblocks the scanner goroutine before the test exits.
			_ = resp.Body.Close()
			t.Fatal("did not receive event over SSE within deadline")
		case line, ok := <-lines:
			if !ok {
				_ = resp.Body.Close()
				t.Fatal("SSE stream ended before event arrived")
			}
			if strings.HasPrefix(line, "data: ") && strings.Contains(line, "\"kind\":\"x\"") {
				_ = resp.Body.Close()
				return
			}
		}
	}
}
