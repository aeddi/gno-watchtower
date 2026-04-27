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

//go:embed chain_block_committed.md
var chainBlockCommittedDoc string

// BlockCommitted handles "Executed block" log lines and emits chain.block_committed.
// The proposer field is not present in the slog line; it is left empty pending
// enrichment from RPC data or Phase-14 replay test validation.
type BlockCommitted struct{ cluster string }

// NewBlockCommitted returns a BlockCommitted handler for the given cluster.
func NewBlockCommitted(cluster string) *BlockCommitted { return &BlockCommitted{cluster: cluster} }

func (BlockCommitted) Name() string { return "block_committed" }

func (BlockCommitted) Meta() handlers.Meta {
	return handlers.Meta{
		Kind:        "chain.block_committed",
		Source:      handlers.SourceLog,
		Description: "A block was executed and committed to the chain.",
		DocRef:      "/docs/handlers/chain.block_committed",
	}
}

func (h *BlockCommitted) Handle(_ context.Context, o normalizer.Observation) []types.Op {
	if o.LogEntry == nil {
		return nil
	}
	m, ok := matchLog(o.LogEntry.Line, "Executed block")
	if !ok {
		return nil
	}
	height := readInt64(m, "height")
	validTxs := readInt64(m, "validTxs")
	invalidTxs := readInt64(m, "invalidTxs")
	txs := int32(validTxs + invalidTxs)
	payload := sk.ChainBlockCommitted{Height: height, Txs: txs}
	pb, _ := json.Marshal(payload)
	ev := types.Event{
		EventID:    eventid.Derive(o.LogEntry.Time, payload.Kind(), types.SubjectChain, pb),
		ClusterID:  h.cluster,
		Time:       o.LogEntry.Time,
		IngestTime: o.IngestTime,
		Kind:       payload.Kind(),
		Subject:    types.SubjectChain,
		Payload:    map[string]any{"height": height, "txs": txs},
		Provenance: provenanceFromEntry(o),
	}
	return []types.Op{{Kind: types.OpInsertEvent, Event: &ev}}
}

func init() {
	handlers.Register("chain.block_committed",
		func(cluster string) handlers.Handler { return NewBlockCommitted(cluster) },
		chainBlockCommittedDoc)
}
