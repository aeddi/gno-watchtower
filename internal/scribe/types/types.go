package types

import "time"

// SubjectChain is the canonical subject value for chain-level events/anchors.
const SubjectChain = "_chain"

// ProvenanceType enumerates the three provenance flavors per the spec.
type ProvenanceType string

const (
	ProvenanceLog     ProvenanceType = "log"
	ProvenanceMetric  ProvenanceType = "metric"
	ProvenanceDerived ProvenanceType = "derived"
)

// Provenance records how scribe knows an event happened.
// JSON-marshalled into the events.provenance column.
type Provenance struct {
	Type    ProvenanceType `json:"type"`
	Query   string         `json:"query,omitempty"`
	Rule    string         `json:"rule,omitempty"`
	LogRefs []LogRef       `json:"log_refs,omitempty"`
	Metric  *MetricRef     `json:"metric,omitempty"`
	Inputs  []DerivationIn `json:"inputs,omitempty"`
	Queries []ProvenanceQ  `json:"queries,omitempty"`
}

type LogRef struct {
	StreamLabels map[string]string `json:"stream_labels"`
	LineTime     time.Time         `json:"line_time"`
	LineHash     string            `json:"line_hash"`
}

type MetricRef struct {
	Backend string    `json:"backend"`
	Query   string    `json:"query"`
	Value   float64   `json:"value"`
	At      time.Time `json:"at"`
}

// DerivationIn is one input to a derived event. Either EventID OR AbsenceOf is set.
type DerivationIn struct {
	EventID   string         `json:"event_id,omitempty"`
	Kind      string         `json:"kind,omitempty"`
	AbsenceOf string         `json:"absence_of,omitempty"`
	Subject   string         `json:"subject,omitempty"`
	Extras    map[string]any `json:"extras,omitempty"`
}

type ProvenanceQ struct {
	Backend string `json:"backend"`
	Q       string `json:"q"`
}

// OpKind enumerates writer queue message kinds.
type OpKind int

const (
	OpInsertEvent OpKind = iota
	OpUpsertSampleValidator
	OpUpsertSampleChain
	OpInsertAnchor
)

func (k OpKind) String() string {
	switch k {
	case OpInsertEvent:
		return "insert_event"
	case OpUpsertSampleValidator:
		return "upsert_sample_validator"
	case OpUpsertSampleChain:
		return "upsert_sample_chain"
	case OpInsertAnchor:
		return "insert_anchor"
	default:
		return "unknown"
	}
}

// Op is a writer queue message. Exactly one of Event / SampleValid / SampleChain
// / Anchor is populated based on Kind; the writer dispatches on Kind.
type Op struct {
	Kind         OpKind
	Event        *Event
	SampleValid  *SampleValidator
	SampleChain  *SampleChain
	Anchor       *Anchor
	FromBackfill bool // priority shim: live=false drains first
}

// Forward declarations — fields populated in Task 1.2.
type (
	Event           struct{}
	SampleValidator struct{}
	SampleChain     struct{}
	Anchor          struct{}
)
