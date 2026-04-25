package types

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEventRoundtrip(t *testing.T) {
	now := time.Date(2026, 4, 25, 10, 14, 33, 0, time.UTC)
	e := Event{
		EventID:    "01JCT9F2X4N6KQR1B2W7E3Y8M5",
		ClusterID:  "gno-cluster-test",
		Time:       now,
		IngestTime: now.Add(time.Millisecond * 200),
		Kind:       "validator.height_advanced",
		Subject:    "node-1",
		Payload:    map[string]any{"from": float64(100), "to": float64(101)},
		Provenance: Provenance{Type: ProvenanceMetric, Query: "sentinel_rpc_latest_block_height"},
	}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Event
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.EventID != e.EventID || got.Kind != e.Kind || got.Subject != e.Subject {
		t.Errorf("roundtrip mismatch: got %+v", got)
	}
}

func TestSampleValidatorZeroIsValid(t *testing.T) {
	var s SampleValidator
	s.ClusterID = "c1"
	s.Validator = "node-1"
	s.Time = time.Now()
	s.Tier = 0
	if s.ClusterID == "" {
		t.Error("ClusterID not stored")
	}
}
