package analysis

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/eventid"
	"github.com/aeddi/gno-watchtower/internal/scribe/scribemetrics"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

// writerSubmitter is the narrow surface of *writer.Writer the analysis engine
// uses. Defined here as an interface so tests can swap in a fake.
type writerSubmitter interface {
	Submit(op types.Op)
}

// newEmitter returns an Emitter that wraps every Diagnostic into a types.Event
// with kind = meta.Kind() and analysis columns populated, then routes through
// w.Submit. Bumps scribe_analysis_emissions_total per emission.
//
// Recovered diagnostics with an empty Recovers field are dropped (logged +
// scribe_analysis_emit_errors_total incremented) — emitting one would leave
// an orphaned recovered row that doesn't pair with any open.
func newEmitter(meta Meta, clusterID string, w writerSubmitter, m *scribemetrics.Registry, now func() time.Time) Emitter {
	return func(d Diagnostic) {
		state := d.State
		if state == "" {
			state = StateOpen
		}
		if state == StateRecovered && d.Recovers == "" {
			slog.Error("analysis: rule emitted state=recovered with empty Recovers; dropping",
				"rule", meta.Kind(), "subject", d.Subject)
			if m != nil && m.AnalysisEmitErrors != nil {
				m.AnalysisEmitErrors.WithLabelValues(meta.Kind()).Inc()
			}
			return
		}
		sev := d.Severity
		if sev == "" {
			sev = meta.Severity
		}

		t := now().UTC()
		payload := d.Payload
		if payload == nil {
			payload = map[string]any{}
		}
		payloadBytes, _ := json.Marshal(payload)

		ev := types.Event{
			EventID:    eventid.Derive(t, meta.Kind(), d.Subject, payloadBytes),
			ClusterID:  clusterID,
			Time:       t,
			IngestTime: t,
			Kind:       meta.Kind(),
			Subject:    d.Subject,
			Severity:   string(sev),
			State:      string(state),
			Recovers:   d.Recovers,
			Payload:    payload,
			Provenance: types.Provenance{
				Type:          types.ProvenanceRule,
				Rule:          meta.Kind(),
				DocRef:        "/docs/rules/" + meta.Kind(),
				LinkedSignals: d.LinkedSignals,
			},
		}
		w.Submit(types.Op{Kind: types.OpInsertEvent, Event: &ev})
		if m != nil && m.AnalysisEmissions != nil {
			m.AnalysisEmissions.WithLabelValues(meta.Kind(), string(sev), string(state)).Inc()
		}
	}
}
