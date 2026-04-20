// pkg/protocol/metrics_test.go
package protocol_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/pkg/protocol"
)

func TestMetricsPayload_RoundTrip(t *testing.T) {
	p := protocol.MetricsPayload{
		CollectedAt: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		Data: map[string]json.RawMessage{
			"cpu":    json.RawMessage(`{"percent":12.5}`),
			"memory": json.RawMessage(`{"used":1024}`),
		},
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got protocol.MetricsPayload
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.CollectedAt.Equal(p.CollectedAt) {
		t.Errorf("CollectedAt: got %v, want %v", got.CollectedAt, p.CollectedAt)
	}
	if len(got.Data) != 2 {
		t.Fatalf("Data len: got %d, want 2", len(got.Data))
	}
	if string(got.Data["cpu"]) != `{"percent":12.5}` {
		t.Errorf("Data[cpu]: got %s", got.Data["cpu"])
	}
}

func TestMetricsPayload_EmptyData(t *testing.T) {
	p := protocol.MetricsPayload{
		CollectedAt: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		Data:        map[string]json.RawMessage{},
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(b) != `{"collected_at":"2026-04-01T00:00:00Z","data":{}}` {
		t.Errorf("unexpected JSON: %s", b)
	}
}
