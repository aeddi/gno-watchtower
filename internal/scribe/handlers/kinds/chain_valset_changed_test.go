package kinds_test

import (
	"testing"

	"github.com/aeddi/gno-watchtower/internal/scribe/handlers/kinds"
)

func TestValsetChangedHandler(t *testing.T) {
	ops := runLogHandler(t, kinds.NewValsetChanged("c1"), "chain.valset_changed.jsonl")
	if len(ops) != 1 || ops[0].Event.Kind != "chain.valset_changed" {
		t.Fatalf("got %+v", ops)
	}
}
