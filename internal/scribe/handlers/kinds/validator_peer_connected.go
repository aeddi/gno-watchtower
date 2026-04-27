package kinds

import (
	"context"
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/aeddi/gno-watchtower/internal/scribe/eventid"
	"github.com/aeddi/gno-watchtower/internal/scribe/handlers"
	sk "github.com/aeddi/gno-watchtower/internal/scribe/kinds"
	"github.com/aeddi/gno-watchtower/internal/scribe/normalizer"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

//go:embed validator_peer_connected.md
var validatorPeerConnectedDoc string

// PeerConnected handles "Added peer" log lines and emits validator.peer_connected.
type PeerConnected struct{ cluster string }

// NewPeerConnected returns a PeerConnected handler for the given cluster.
func NewPeerConnected(cluster string) *PeerConnected { return &PeerConnected{cluster: cluster} }

func (PeerConnected) Name() string { return "peer_connected" }

func (PeerConnected) Meta() handlers.Meta {
	return handlers.Meta{
		Kind:        "validator.peer_connected",
		Source:      handlers.SourceLog,
		Description: "A new outbound peer connection was established by the validator.",
		DocRef:      "/docs/handlers/validator.peer_connected",
	}
}

func (h *PeerConnected) Handle(_ context.Context, o normalizer.Observation) []types.Op {
	if o.LogEntry == nil {
		return nil
	}
	m, ok := matchLog(o.LogEntry.Line, "Added peer")
	if !ok {
		return nil
	}
	peer, _ := m["peer"].(string)
	// Derive peer_id as the part before '@' if present, else the full value.
	peerID := peer
	if at := strings.IndexByte(peer, '@'); at >= 0 {
		peerID = peer[:at]
	}
	val := o.LogEntry.Stream.Labels["validator"]
	payload := sk.ValidatorPeerConnected{Peer: peer, PeerID: peerID, Direction: "out"}
	pb, _ := json.Marshal(payload)
	ev := types.Event{
		EventID:    eventid.Derive(o.LogEntry.Time, payload.Kind(), val, pb),
		ClusterID:  h.cluster,
		Time:       o.LogEntry.Time,
		IngestTime: o.IngestTime,
		Kind:       payload.Kind(),
		Subject:    val,
		Payload:    map[string]any{"peer": peer, "peer_id": peerID, "direction": "out"},
		Provenance: provenanceFromEntry(o),
	}
	return []types.Op{{Kind: types.OpInsertEvent, Event: &ev}}
}

func init() {
	handlers.Register("validator.peer_connected",
		func(cluster string) handlers.Handler { return NewPeerConnected(cluster) },
		validatorPeerConnectedDoc)
}
