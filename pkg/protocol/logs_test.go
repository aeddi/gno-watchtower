package protocol_test

import (
	"encoding/json"
	"testing"

	"github.com/aeddi/gno-watchtower/pkg/protocol"
)

func TestLogPayload_RoundTrip(t *testing.T) {
	p := protocol.LogPayload{
		Level: "warn",
		Lines: []json.RawMessage{
			json.RawMessage(`{"level":"warn","msg":"test","height":42}`),
		},
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got protocol.LogPayload
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Level != "warn" {
		t.Errorf("Level: got %q, want %q", got.Level, "warn")
	}
	if len(got.Lines) != 1 {
		t.Fatalf("Lines len: got %d, want 1", len(got.Lines))
	}
	if string(got.Lines[0]) != string(p.Lines[0]) {
		t.Errorf("Lines[0]: got %s, want %s", got.Lines[0], p.Lines[0])
	}
}

func TestLogPayload_EmptyLines(t *testing.T) {
	p := protocol.LogPayload{Level: "info", Lines: []json.RawMessage{}}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(b) != `{"level":"info","lines":[]}` {
		t.Errorf("unexpected JSON: %s", b)
	}
}
