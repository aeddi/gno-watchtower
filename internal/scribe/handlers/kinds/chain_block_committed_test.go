package kinds_test

import (
	"testing"

	"github.com/aeddi/gno-watchtower/internal/scribe/handlers/kinds"
)

func TestBlockCommittedHandler(t *testing.T) {
	ops := runLogHandler(t, kinds.NewBlockCommitted("c1"), "chain.block_committed.jsonl")
	if len(ops) != 1 || ops[0].Event.Kind != "chain.block_committed" {
		t.Fatalf("got %+v", ops)
	}
}
