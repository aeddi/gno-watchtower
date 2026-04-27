package kinds_test

import (
	"testing"

	"github.com/aeddi/gno-watchtower/internal/scribe/handlers/kinds"
)

func TestPeerConnectedHandler(t *testing.T) {
	ops := runLogHandler(t, kinds.NewPeerConnected("c1"), "validator.peer_connected.jsonl")
	if len(ops) != 1 || ops[0].Event.Kind != "validator.peer_connected" {
		t.Fatalf("got %+v", ops)
	}
}
