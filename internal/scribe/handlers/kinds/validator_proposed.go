package kinds

import (
	"context"
	_ "embed"
	"encoding/json"

	"github.com/aeddi/gno-watchtower/internal/scribe/eventid"
	"github.com/aeddi/gno-watchtower/internal/scribe/handlers"
	sk "github.com/aeddi/gno-watchtower/internal/scribe/kinds"
	"github.com/aeddi/gno-watchtower/internal/scribe/normalizer"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

//go:embed validator_proposed.md
var validatorProposedDoc string

// Proposed handles "Our turn to propose" log lines and emits validator.proposed.
type Proposed struct{ cluster string }

// NewProposed returns a Proposed handler for the given cluster.
func NewProposed(cluster string) *Proposed { return &Proposed{cluster: cluster} }

func (Proposed) Name() string { return "proposed" }

func (Proposed) Meta() handlers.Meta {
	return handlers.Meta{
		Kind:        "validator.proposed",
		Source:      handlers.SourceLog,
		Description: "Validator was selected as proposer for a consensus round.",
		DocRef:      "/docs/handlers/validator.proposed",
	}
}

func (h *Proposed) Handle(_ context.Context, o normalizer.Observation) []types.Op {
	if o.LogEntry == nil {
		return nil
	}
	m, ok := matchLog(o.LogEntry.Line, "Our turn to propose")
	if !ok {
		return nil
	}
	height := readInt64(m, "height")
	round := readInt32(m, "round")
	val := o.LogEntry.Stream.Labels["validator"]
	payload := sk.ValidatorProposed{Height: height, Round: round}
	pb, _ := json.Marshal(payload)
	ev := types.Event{
		EventID:    eventid.Derive(o.LogEntry.Time, payload.Kind(), val, pb),
		ClusterID:  h.cluster,
		Time:       o.LogEntry.Time,
		IngestTime: o.IngestTime,
		Kind:       payload.Kind(),
		Subject:    val,
		Payload:    map[string]any{"height": height, "round": round},
		Provenance: provenanceFromEntry(o),
	}
	return []types.Op{{Kind: types.OpInsertEvent, Event: &ev}}
}

func init() {
	handlers.Register("validator.proposed",
		func(cluster string) handlers.Handler { return NewProposed(cluster) },
		validatorProposedDoc)
}
