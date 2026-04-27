package kinds_test

import (
	"testing"

	"github.com/aeddi/gno-watchtower/internal/scribe/handlers/kinds"
)

func TestTxExecutedHandler(t *testing.T) {
	ops := runLogHandler(t, kinds.NewTxExecuted("c1"), "chain.tx_executed.jsonl")
	if len(ops) != 1 || ops[0].Event.Kind != "chain.tx_executed" {
		t.Fatalf("got %+v", ops)
	}
}
