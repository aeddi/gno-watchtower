package cache

import (
	"testing"

	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

func TestCacheSetGet(t *testing.T) {
	c := New()
	state := State{
		Peers:      map[string]types.Provenance{}, // arbitrary structured state
		ValsetView: []map[string]any{{"address": "abc", "voting_power": int64(10)}},
		ConfigHash: "h1",
	}
	c.Put("c1", "node-1", state, "01JCT0AAA0AAA0AAA0AAA0AAA0")
	got, ok := c.Get("c1", "node-1")
	if !ok || got.ConfigHash != "h1" {
		t.Errorf("got=%+v ok=%v", got, ok)
	}
}

func TestSubjectsLists(t *testing.T) {
	c := New()
	c.Put("c1", "node-1", State{}, "")
	c.Put("c1", "node-2", State{}, "")
	c.Put("c1", "_chain", State{}, "")
	subs := c.Subjects("c1")
	if len(subs) != 3 {
		t.Errorf("subs=%v", subs)
	}
}
