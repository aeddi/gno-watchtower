package protocol_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/pkg/protocol"
)

func TestRPCPayload_RoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	raw := json.RawMessage(`{"height":"42"}`)
	p := protocol.RPCPayload{
		CollectedAt: now,
		Data:        map[string]json.RawMessage{"status": raw},
	}

	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got protocol.RPCPayload
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !got.CollectedAt.Equal(now) {
		t.Errorf("CollectedAt: got %v, want %v", got.CollectedAt, now)
	}
	if string(got.Data["status"]) != string(raw) {
		t.Errorf("Data[status]: got %s, want %s", got.Data["status"], raw)
	}
}

func TestRPCPayload_EmptyData(t *testing.T) {
	p := protocol.RPCPayload{
		CollectedAt: time.Now().UTC(),
		Data:        map[string]json.RawMessage{},
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Empty map must marshal as {} — not omitted, not null.
	if !bytes.Contains(b, []byte(`"data":{}`)) {
		t.Errorf("expected data:{} in JSON, got: %s", b)
	}
}

func TestRPCPayload_NilData(t *testing.T) {
	p := protocol.RPCPayload{CollectedAt: time.Now().UTC()}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// nil map marshals as null — callers must always initialise Data.
	if !bytes.Contains(b, []byte(`"data":null`)) {
		t.Errorf("expected data:null in JSON for nil map, got: %s", b)
	}
}
