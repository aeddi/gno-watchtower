package kinds_test

import (
	"testing"

	"github.com/aeddi/gno-watchtower/internal/scribe/handlers/kinds"
)

func TestPeerDisconnectedHandler(t *testing.T) {
	ops := runLogHandler(t, kinds.NewPeerDisconnected("c1"), "validator.peer_disconnected.jsonl")
	if len(ops) != 1 || ops[0].Event.Kind != "validator.peer_disconnected" {
		t.Fatalf("got %+v", ops)
	}
}
