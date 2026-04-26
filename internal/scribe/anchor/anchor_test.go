package anchor

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/cache"
	"github.com/aeddi/gno-watchtower/internal/scribe/store"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
	"github.com/aeddi/gno-watchtower/internal/scribe/writer"
)

func TestAnchorWriterPersistsCacheState(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "scribe.duckdb"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	c := cache.New()
	c.Put("c1", "node-1", cache.State{ConfigHash: "h1"}, "01JCT0AAA0AAA0AAA0AAA0AAA0")

	w := writer.New(s, writer.Config{BatchSize: 1, BatchWindow: 20 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = w.Run(ctx) }()

	a := New(c, w, "c1")
	a.WriteOnce(ctx, time.Now())

	time.Sleep(150 * time.Millisecond)

	got, err := s.GetLatestAnchor(ctx, "c1", "node-1", time.Now())
	if err != nil || got == nil {
		t.Fatalf("anchor missing: %v %v", got, err)
	}
	if got.EventsThrough != "01JCT0AAA0AAA0AAA0AAA0AAA0" {
		t.Errorf("events_through = %q", got.EventsThrough)
	}
	_ = types.Anchor{} // keep import
}
