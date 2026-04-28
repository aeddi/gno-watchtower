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
	payload := ops[0].Event.Payload
	// peer_id should be the bare node_id, regardless of log format.
	if got := payload["peer_id"]; got != "g1peer567890" {
		t.Errorf("peer_id = %v, want g1peer567890", got)
	}
	// peer_subject is empty without a resolver.
	if got, ok := payload["peer_subject"].(string); ok && got != "" {
		t.Errorf("peer_subject = %q, want empty (no resolver)", got)
	}
}

func TestPeerConnectedHandler_WithResolver(t *testing.T) {
	h := kinds.NewPeerConnected("c1")
	h.SetPeerResolver(kinds.NewMapResolver(map[string]string{
		"g1peer567890": "node-2",
	}))
	ops := runLogHandler(t, h, "validator.peer_connected.jsonl")
	if len(ops) != 1 {
		t.Fatalf("got %d ops, want 1", len(ops))
	}
	payload := ops[0].Event.Payload
	if got := payload["peer_subject"]; got != "node-2" {
		t.Errorf("peer_subject = %v, want node-2", got)
	}
	if got := payload["peer_id"]; got != "g1peer567890" {
		t.Errorf("peer_id = %v, want g1peer567890", got)
	}
}
