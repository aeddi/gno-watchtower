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

//go:embed validator_peer_disconnected.md
var validatorPeerDisconnectedDoc string

// PeerDisconnected handles "Stopping peer for error" log lines and emits
// validator.peer_disconnected.
type PeerDisconnected struct {
	cluster  string
	resolver PeerResolver
}

// NewPeerDisconnected returns a PeerDisconnected handler for the given cluster.
func NewPeerDisconnected(cluster string) *PeerDisconnected {
	return &PeerDisconnected{cluster: cluster}
}

// SetPeerResolver injects an optional resolver that maps the peer's node_id to
// its validator subject moniker so the UI can attribute disconnects to a known
// node.
func (h *PeerDisconnected) SetPeerResolver(r PeerResolver) { h.resolver = r }

func (PeerDisconnected) Name() string { return "peer_disconnected" }

func (PeerDisconnected) Meta() handlers.Meta {
	return handlers.Meta{
		Kind:        "validator.peer_disconnected",
		Source:      handlers.SourceLog,
		Description: "A peer connection was dropped due to an error.",
		DocRef:      "/docs/handlers/validator.peer_disconnected",
	}
}

func (h *PeerDisconnected) Handle(_ context.Context, o normalizer.Observation) []types.Op {
	if o.LogEntry == nil {
		return nil
	}
	m, ok := matchLog(o.LogEntry.Line, "Stopping peer for error")
	if !ok {
		return nil
	}
	peer, _ := m["peer"].(string)
	peerID := ExtractNodeID(peer)
	if peerID == "" {
		peerID = peer
	}
	peerSubject := ""
	if h.resolver != nil {
		peerSubject = h.resolver.Resolve(peer)
	}
	reason, _ := m["err"].(string)
	val := o.LogEntry.Stream.Labels["validator"]
	payload := sk.ValidatorPeerDisconnected{Peer: peerID, Reason: reason}
	pb, _ := json.Marshal(payload)
	ev := types.Event{
		EventID:    eventid.Derive(o.LogEntry.Time, payload.Kind(), val, pb),
		ClusterID:  h.cluster,
		Time:       o.LogEntry.Time,
		IngestTime: o.IngestTime,
		Kind:       payload.Kind(),
		Subject:    val,
		Payload: map[string]any{
			"peer_id":      peerID,
			"peer_subject": peerSubject,
			"reason":       reason,
		},
		Provenance: provenanceFromEntry(o),
	}
	return []types.Op{{Kind: types.OpInsertEvent, Event: &ev}}
}

func init() {
	handlers.Register("validator.peer_disconnected",
		func(cluster string) handlers.Handler { return NewPeerDisconnected(cluster) },
		validatorPeerDisconnectedDoc)
}
