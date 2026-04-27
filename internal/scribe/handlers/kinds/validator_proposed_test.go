package kinds_test

import (
	"testing"

	"github.com/aeddi/gno-watchtower/internal/scribe/handlers/kinds"
)

func TestProposedHandler(t *testing.T) {
	ops := runLogHandler(t, kinds.NewProposed("c1"), "validator.proposed.jsonl")
	if len(ops) != 1 || ops[0].Event.Kind != "validator.proposed" {
		t.Fatalf("got %+v", ops)
	}
}
