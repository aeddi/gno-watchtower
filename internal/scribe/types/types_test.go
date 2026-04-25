package types

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSubjectChainConstant(t *testing.T) {
	if SubjectChain != "_chain" {
		t.Errorf("SubjectChain = %q, want %q", SubjectChain, "_chain")
	}
}

func TestProvenanceLogJSONShape(t *testing.T) {
	p := Provenance{
		Type:  ProvenanceLog,
		Query: "{validator=\"node-1\"} |= \"Precommit\"",
		LogRefs: []LogRef{{
			StreamLabels: map[string]string{"validator": "node-1", "level": "info"},
			LineTime:     time.Date(2026, 4, 25, 10, 14, 33, 0, time.UTC),
			LineHash:     "abc123",
		}},
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["type"] != "log" {
		t.Errorf("type = %v, want \"log\"", got["type"])
	}
}

func TestOpKindStrings(t *testing.T) {
	cases := []struct {
		k    OpKind
		want string
	}{
		{OpInsertEvent, "insert_event"},
		{OpUpsertSampleValidator, "upsert_sample_validator"},
		{OpUpsertSampleChain, "upsert_sample_chain"},
		{OpInsertAnchor, "insert_anchor"},
	}
	for _, c := range cases {
		if c.k.String() != c.want {
			t.Errorf("OpKind %d.String() = %q, want %q", c.k, c.k.String(), c.want)
		}
	}
}
