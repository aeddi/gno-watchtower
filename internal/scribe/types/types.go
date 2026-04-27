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
	ProvenanceRule    ProvenanceType = "rule"
)

// SignalLink is a resolved (placeholders filled) Loki/VM query the rule built
// for a specific diagnostic instance. Stored inside Provenance.LinkedSignals.
type SignalLink struct {
	Type  string    `json:"type"` // "loki" | "vm"
	Query string    `json:"query"`
	URL   string    `json:"url,omitempty"`
	From  time.Time `json:"from"`
	To    time.Time `json:"to"`
}

// Provenance records how scribe knows an event happened.
// JSON-marshalled into the events.provenance column.
type Provenance struct {
	Type           ProvenanceType `json:"type"`
	Query          string         `json:"query,omitempty"`
	Rule           string         `json:"rule,omitempty"`
	DocRef         string         `json:"doc_ref,omitempty"`
	SourceEventIDs []string       `json:"source_event_ids,omitempty"`
	LinkedSignals  []SignalLink   `json:"linked_signals,omitempty"`
	LogRefs        []LogRef       `json:"log_refs,omitempty"`
	Metric         *MetricRef     `json:"metric,omitempty"`
	Inputs         []DerivationIn `json:"inputs,omitempty"`
	Queries        []ProvenanceQ  `json:"queries,omitempty"`
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

// Event mirrors a row in the events table.
type Event struct {
	EventID    string         `json:"event_id"`
	ClusterID  string         `json:"cluster_id"`
	Time       time.Time      `json:"time"`
	IngestTime time.Time      `json:"ingest_time"`
	Kind       string         `json:"kind"`
	Subject    string         `json:"subject"`
	Severity   string         `json:"severity,omitempty"` // analysis: "warning"|"error"|"critical" or "" for non-diagnostic
	State      string         `json:"state,omitempty"`    // analysis: "open"|"recovered" or ""
	Recovers   string         `json:"recovers,omitempty"` // analysis: event_id of opening event when state="recovered"
	Payload    map[string]any `json:"payload"`
	Provenance Provenance     `json:"provenance"`
}

// SampleValidator mirrors a row in the samples_validator table. Pointer fields
// represent NULL when nil (only populated on warm rollup).
type SampleValidator struct {
	ClusterID       string
	Validator       string
	Time            time.Time
	Tier            int8
	Height          int64
	VotingPower     int64
	CatchingUp      bool
	MempoolTxs      int32
	MempoolTxsMax   *int32
	MempoolCached   int32
	CPUPct          float32
	CPUPctMax       *float32
	MemPct          float32
	MemPctMax       *float32
	DiskPct         float32
	NetRxBps        float32
	NetTxBps        float32
	PeerCountIn     int16
	PeerCountInMin  *int16
	PeerCountOut    int16
	PeerCountOutMin *int16
	BehindSentry    *bool
	LastObserved    time.Time
}

// SampleChain mirrors a row in the samples_chain table.
type SampleChain struct {
	ClusterID        string
	Time             time.Time
	Tier             int8
	BlockHeight      int64
	OnlineCount      int16
	OnlineCountMin   *int16
	CatchingUpCount  int16
	ValsetSize       int16
	TotalVotingPower int64
}

// Anchor mirrors a row in the state_anchors table.
type Anchor struct {
	ClusterID     string         `json:"cluster_id"`
	Subject       string         `json:"subject"`
	Time          time.Time      `json:"t"`
	FullState     map[string]any `json:"full_state"`
	EventsThrough string         `json:"events_through"` // last event_id covered
}
