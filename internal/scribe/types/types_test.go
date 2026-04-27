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

func TestProvenanceJSONRoundtripIncludesAnalysisFields(t *testing.T) {
	p := Provenance{
		Type:           ProvenanceRule,
		Rule:           "block_missed_v1",
		DocRef:         "/docs/rules/block_missed_v1",
		SourceEventIDs: []string{"01JCV0BNAQQNKDPQ95R5RAWAZ7"},
		LinkedSignals: []SignalLink{{
			Type:  "loki",
			Query: `{validator="moul-3"}`,
			URL:   "https://grafana.example/explore",
			From:  time.Unix(0, 0).UTC(),
			To:    time.Unix(60, 0).UTC(),
		}},
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Provenance
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Rule != "block_missed_v1" || got.DocRef != "/docs/rules/block_missed_v1" {
		t.Errorf("rule/doc_ref roundtrip failed: %+v", got)
	}
	if len(got.SourceEventIDs) != 1 || got.SourceEventIDs[0] != "01JCV0BNAQQNKDPQ95R5RAWAZ7" {
		t.Errorf("source_event_ids roundtrip failed: %+v", got)
	}
	if len(got.LinkedSignals) != 1 || got.LinkedSignals[0].Type != "loki" {
		t.Errorf("linked_signals roundtrip failed: %+v", got)
	}
}

func TestEventCarriesAnalysisColumnsAsZeroForNonDiagnostic(t *testing.T) {
	e := Event{Kind: "validator.vote_cast"}
	if e.Severity != "" || e.State != "" || e.Recovers != "" {
		t.Errorf("non-diagnostic event must have empty severity/state/recovers, got %+v", e)
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
