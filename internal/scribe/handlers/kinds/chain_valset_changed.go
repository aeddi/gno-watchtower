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

//go:embed chain_valset_changed.md
var chainValsetChangedDoc string

// ValsetChanged handles "Updates to validators" log lines and emits
// chain.valset_changed.
type ValsetChanged struct{ cluster string }

// NewValsetChanged returns a ValsetChanged handler for the given cluster.
func NewValsetChanged(cluster string) *ValsetChanged { return &ValsetChanged{cluster: cluster} }

func (ValsetChanged) Name() string { return "valset_changed" }

func (ValsetChanged) Meta() handlers.Meta {
	return handlers.Meta{
		Kind:        "chain.valset_changed",
		Source:      handlers.SourceLog,
		Description: "The active validator set was updated (additions logged; removals pending phase-14 data).",
		DocRef:      "/docs/handlers/chain.valset_changed",
	}
}

func (h *ValsetChanged) Handle(_ context.Context, o normalizer.Observation) []types.Op {
	if o.LogEntry == nil {
		return nil
	}
	m, ok := matchLog(o.LogEntry.Line, "Updates to validators")
	if !ok {
		return nil
	}
	height := readInt64(m, "height")
	// "updates" carries the added validators. gnoland doesn't log removals in
	// the same line; removed is left empty pending Phase-14 replay test validation.
	var added []sk.ValsetMember
	if raw, ok := m["updates"].([]any); ok {
		for _, item := range raw {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			addr, _ := entry["address"].(string)
			power := readInt64(entry, "power")
			added = append(added, sk.ValsetMember{Address: addr, VotingPower: power})
		}
	}
	payload := sk.ChainValsetChanged{Height: height, Added: added, Removed: nil}
	pb, _ := json.Marshal(payload)
	ev := types.Event{
		EventID:    eventid.Derive(o.LogEntry.Time, payload.Kind(), types.SubjectChain, pb),
		ClusterID:  h.cluster,
		Time:       o.LogEntry.Time,
		IngestTime: o.IngestTime,
		Kind:       payload.Kind(),
		Subject:    types.SubjectChain,
		Payload:    map[string]any{"height": height, "added": added, "removed": nil},
		Provenance: provenanceFromEntry(o),
	}
	return []types.Op{{Kind: types.OpInsertEvent, Event: &ev}}
}

func init() {
	handlers.Register("chain.valset_changed",
		func(cluster string) handlers.Handler { return NewValsetChanged(cluster) },
		chainValsetChangedDoc)
}
