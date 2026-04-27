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

//go:embed chain_tx_executed.md
var chainTxExecutedDoc string

// TxExecuted is a best-effort handler for chain.tx_executed events. It matches
// "Rejected bad transaction" (failure path). gnoland does not emit a structured
// per-transaction success log line; the success path is not implemented here.
// Phase-14 replay tests will validate and may prompt a revision of this handler.
type TxExecuted struct{ cluster string }

// NewTxExecuted returns a TxExecuted handler for the given cluster.
func NewTxExecuted(cluster string) *TxExecuted { return &TxExecuted{cluster: cluster} }

func (TxExecuted) Name() string { return "tx_executed" }

func (TxExecuted) Meta() handlers.Meta {
	return handlers.Meta{
		Kind:        "chain.tx_executed",
		Source:      handlers.SourceLog,
		Description: "A transaction was executed; currently captures only the rejection (failure) path.",
		DocRef:      "/docs/handlers/chain.tx_executed",
	}
}

// Handle emits a chain.tx_executed Op for rejected transactions.
// TODO(phase-14): add success path once real log lines are validated.
func (h *TxExecuted) Handle(_ context.Context, o normalizer.Observation) []types.Op {
	if o.LogEntry == nil {
		return nil
	}
	m, ok := matchLog(o.LogEntry.Line, "Rejected bad transaction")
	if !ok {
		return nil
	}
	height := readInt64(m, "height")
	errMsg, _ := m["err"].(string)
	payload := sk.ChainTxExecuted{
		Height:  height,
		Type:    "unknown",
		Success: false,
		Error:   errMsg,
	}
	pb, _ := json.Marshal(payload)
	ev := types.Event{
		EventID:    eventid.Derive(o.LogEntry.Time, payload.Kind(), types.SubjectChain, pb),
		ClusterID:  h.cluster,
		Time:       o.LogEntry.Time,
		IngestTime: o.IngestTime,
		Kind:       payload.Kind(),
		Subject:    types.SubjectChain,
		Payload:    map[string]any{"height": height, "type": "unknown", "success": false, "error": errMsg},
		Provenance: provenanceFromEntry(o),
	}
	return []types.Op{{Kind: types.OpInsertEvent, Event: &ev}}
}

func init() {
	handlers.Register("chain.tx_executed",
		func(cluster string) handlers.Handler { return NewTxExecuted(cluster) },
		chainTxExecutedDoc)
}
