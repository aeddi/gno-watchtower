package handlers

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/aeddi/gno-watchtower/internal/scribe/eventid"
	"github.com/aeddi/gno-watchtower/internal/scribe/kinds"
	"github.com/aeddi/gno-watchtower/internal/scribe/normalizer"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

// matchLog decodes a slog JSON line into a flat map and checks that msg contains
// the given marker. Returns the map and true when matched.
func matchLog(line, msgContains string) (map[string]any, bool) {
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &m); err != nil {
		return nil, false
	}
	msg, _ := m["msg"].(string)
	if !strings.Contains(msg, msgContains) {
		return nil, false
	}
	return m, true
}

// provenanceFromEntry builds a ProvenanceLog Provenance from a log observation.
func provenanceFromEntry(o normalizer.Observation) types.Provenance {
	stream := make(map[string]string, len(o.LogEntry.Stream.Labels))
	for k, v := range o.LogEntry.Stream.Labels {
		stream[k] = v
	}
	hash := sha1.Sum([]byte(o.LogEntry.Line))
	return types.Provenance{
		Type:  types.ProvenanceLog,
		Query: o.LogQuery,
		LogRefs: []types.LogRef{{
			StreamLabels: stream,
			LineTime:     o.LogEntry.Time,
			LineHash:     hex.EncodeToString(hash[:]),
		}},
	}
}

// readInt64 coerces a JSON-decoded field (float64 or int64) to int64.
func readInt64(m map[string]any, k string) int64 {
	switch v := m[k].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	default:
		return 0
	}
}

func readInt32(m map[string]any, k string) int32 { return int32(readInt64(m, k)) }

// ---- Proposed

// Proposed handles "Our turn to propose" log lines and emits validator.proposed.
type Proposed struct{ cluster string }

// NewProposed returns a Proposed handler for the given cluster.
func NewProposed(cluster string) *Proposed { return &Proposed{cluster: cluster} }

// Name returns the handler name.
func (Proposed) Name() string { return "proposed" }

// Handle emits a validator.proposed Op when the log line matches.
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
	payload := kinds.ValidatorProposed{Height: height, Round: round}
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

// ---- ConsensusRoundStep

// rxRoundStep extracts the step name and height/round from enterXxx(h/r) messages.
var rxRoundStep = regexp.MustCompile(`enter(\w+)\((\d+)/(\d+)\)`)

// ConsensusRoundStep handles enterPropose/enterPrevote/enterPrecommit/enterCommit
// log lines and emits validator.consensus.round_step.
type ConsensusRoundStep struct{ cluster string }

// NewConsensusRoundStep returns a ConsensusRoundStep handler for the given cluster.
func NewConsensusRoundStep(cluster string) *ConsensusRoundStep {
	return &ConsensusRoundStep{cluster: cluster}
}

// Name returns the handler name.
func (ConsensusRoundStep) Name() string { return "consensus_round_step" }

// Handle emits a validator.consensus.round_step Op when the log line matches.
func (h *ConsensusRoundStep) Handle(_ context.Context, o normalizer.Observation) []types.Op {
	if o.LogEntry == nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(o.LogEntry.Line)), &m); err != nil {
		return nil
	}
	msg, _ := m["msg"].(string)
	matches := rxRoundStep.FindStringSubmatch(msg)
	if len(matches) != 4 {
		return nil
	}
	step := matches[1]
	height, _ := strconv.ParseInt(matches[2], 10, 64)
	round64, _ := strconv.ParseInt(matches[3], 10, 32)
	round := int32(round64)
	val := o.LogEntry.Stream.Labels["validator"]
	payload := kinds.ValidatorConsensusRoundStep{Height: height, Round: round, Step: step}
	pb, _ := json.Marshal(payload)
	ev := types.Event{
		EventID:    eventid.Derive(o.LogEntry.Time, payload.Kind(), val, pb),
		ClusterID:  h.cluster,
		Time:       o.LogEntry.Time,
		IngestTime: o.IngestTime,
		Kind:       payload.Kind(),
		Subject:    val,
		Payload:    map[string]any{"height": height, "round": round, "step": step},
		Provenance: provenanceFromEntry(o),
	}
	return []types.Op{{Kind: types.OpInsertEvent, Event: &ev}}
}

// ---- VoteCast

// VoteCast handles "Signed proposal" and "Signed and pushed vote" log lines and
// emits validator.vote_cast.
type VoteCast struct{ cluster string }

// NewVoteCast returns a VoteCast handler for the given cluster.
func NewVoteCast(cluster string) *VoteCast { return &VoteCast{cluster: cluster} }

// Name returns the handler name.
func (VoteCast) Name() string { return "vote_cast" }

// Handle emits a validator.vote_cast Op when the log line matches.
func (h *VoteCast) Handle(_ context.Context, o normalizer.Observation) []types.Op {
	if o.LogEntry == nil {
		return nil
	}
	// Match either a signed proposal or a signed+pushed vote.
	line := strings.TrimSpace(o.LogEntry.Line)
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		return nil
	}
	msg, _ := m["msg"].(string)
	if !strings.Contains(msg, "Signed proposal") && !strings.Contains(msg, "Signed and pushed vote") {
		return nil
	}
	height := readInt64(m, "height")
	round := readInt32(m, "round")
	voteType, _ := m["type"].(string)
	if voteType == "" {
		if strings.Contains(msg, "Signed proposal") {
			voteType = "Proposal"
		}
	}
	val := o.LogEntry.Stream.Labels["validator"]
	payload := kinds.ValidatorVoteCast{Height: height, Round: round, VoteType: voteType}
	pb, _ := json.Marshal(payload)
	ev := types.Event{
		EventID:    eventid.Derive(o.LogEntry.Time, payload.Kind(), val, pb),
		ClusterID:  h.cluster,
		Time:       o.LogEntry.Time,
		IngestTime: o.IngestTime,
		Kind:       payload.Kind(),
		Subject:    val,
		Payload:    map[string]any{"height": height, "round": round, "vote_type": voteType},
		Provenance: provenanceFromEntry(o),
	}
	return []types.Op{{Kind: types.OpInsertEvent, Event: &ev}}
}

// ---- PeerConnected

// PeerConnected handles "Added peer" log lines and emits validator.peer_connected.
type PeerConnected struct{ cluster string }

// NewPeerConnected returns a PeerConnected handler for the given cluster.
func NewPeerConnected(cluster string) *PeerConnected { return &PeerConnected{cluster: cluster} }

// Name returns the handler name.
func (PeerConnected) Name() string { return "peer_connected" }

// Handle emits a validator.peer_connected Op when the log line matches.
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
	payload := kinds.ValidatorPeerConnected{Peer: peer, PeerID: peerID, Direction: "out"}
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

// ---- PeerDisconnected

// PeerDisconnected handles "Stopping peer for error" log lines and emits
// validator.peer_disconnected.
type PeerDisconnected struct{ cluster string }

// NewPeerDisconnected returns a PeerDisconnected handler for the given cluster.
func NewPeerDisconnected(cluster string) *PeerDisconnected {
	return &PeerDisconnected{cluster: cluster}
}

// Name returns the handler name.
func (PeerDisconnected) Name() string { return "peer_disconnected" }

// Handle emits a validator.peer_disconnected Op when the log line matches.
func (h *PeerDisconnected) Handle(_ context.Context, o normalizer.Observation) []types.Op {
	if o.LogEntry == nil {
		return nil
	}
	m, ok := matchLog(o.LogEntry.Line, "Stopping peer for error")
	if !ok {
		return nil
	}
	peer, _ := m["peer"].(string)
	peerID := peer
	if at := strings.IndexByte(peer, '@'); at >= 0 {
		peerID = peer[:at]
	}
	reason, _ := m["err"].(string)
	val := o.LogEntry.Stream.Labels["validator"]
	payload := kinds.ValidatorPeerDisconnected{Peer: peerID, Reason: reason}
	pb, _ := json.Marshal(payload)
	ev := types.Event{
		EventID:    eventid.Derive(o.LogEntry.Time, payload.Kind(), val, pb),
		ClusterID:  h.cluster,
		Time:       o.LogEntry.Time,
		IngestTime: o.IngestTime,
		Kind:       payload.Kind(),
		Subject:    val,
		Payload:    map[string]any{"peer_id": peerID, "reason": reason},
		Provenance: provenanceFromEntry(o),
	}
	return []types.Op{{Kind: types.OpInsertEvent, Event: &ev}}
}

// ---- BlockCommitted

// BlockCommitted handles "Executed block" log lines and emits chain.block_committed.
// The proposer field is not present in the slog line; it is left empty pending
// enrichment from RPC data or Phase-14 replay test validation.
type BlockCommitted struct{ cluster string }

// NewBlockCommitted returns a BlockCommitted handler for the given cluster.
func NewBlockCommitted(cluster string) *BlockCommitted { return &BlockCommitted{cluster: cluster} }

// Name returns the handler name.
func (BlockCommitted) Name() string { return "block_committed" }

// Handle emits a chain.block_committed Op when the log line matches.
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
	payload := kinds.ChainBlockCommitted{Height: height, Txs: txs}
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

// ---- ValsetChanged

// ValsetChanged handles "Updates to validators" log lines and emits
// chain.valset_changed.
type ValsetChanged struct{ cluster string }

// NewValsetChanged returns a ValsetChanged handler for the given cluster.
func NewValsetChanged(cluster string) *ValsetChanged { return &ValsetChanged{cluster: cluster} }

// Name returns the handler name.
func (ValsetChanged) Name() string { return "valset_changed" }

// Handle emits a chain.valset_changed Op when the log line matches.
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
	var added []kinds.ValsetMember
	if raw, ok := m["updates"].([]any); ok {
		for _, item := range raw {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			addr, _ := entry["address"].(string)
			power := readInt64(entry, "power")
			added = append(added, kinds.ValsetMember{Address: addr, VotingPower: power})
		}
	}
	payload := kinds.ChainValsetChanged{Height: height, Added: added, Removed: nil}
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

// ---- TxExecuted

// TxExecuted is a best-effort handler for chain.tx_executed events. It matches
// "Rejected bad transaction" (failure path). gnoland does not emit a structured
// per-transaction success log line; the success path is not implemented here.
// Phase-14 replay tests will validate and may prompt a revision of this handler.
type TxExecuted struct{ cluster string }

// NewTxExecuted returns a TxExecuted handler for the given cluster.
func NewTxExecuted(cluster string) *TxExecuted { return &TxExecuted{cluster: cluster} }

// Name returns the handler name.
func (TxExecuted) Name() string { return "tx_executed" }

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
	payload := kinds.ChainTxExecuted{
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
