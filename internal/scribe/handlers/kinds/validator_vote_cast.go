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

//go:embed validator_vote_cast.md
var validatorVoteCastDoc string

// VoteCast handles "Signed proposal" and "Signed and pushed vote" log lines and
// emits validator.vote_cast.
type VoteCast struct{ cluster string }

// NewVoteCast returns a VoteCast handler for the given cluster.
func NewVoteCast(cluster string) *VoteCast { return &VoteCast{cluster: cluster} }

func (VoteCast) Name() string { return "vote_cast" }

func (VoteCast) Meta() handlers.Meta {
	return handlers.Meta{
		Kind:        "validator.vote_cast",
		Source:      handlers.SourceLog,
		Description: "Validator signed and submitted a prevote, precommit, or proposal.",
		DocRef:      "/docs/handlers/validator.vote_cast",
	}
}

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
	payload := sk.ValidatorVoteCast{Height: height, Round: round, VoteType: voteType}
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

func init() {
	handlers.Register("validator.vote_cast",
		func(cluster string) handlers.Handler { return NewVoteCast(cluster) },
		validatorVoteCastDoc)
}
