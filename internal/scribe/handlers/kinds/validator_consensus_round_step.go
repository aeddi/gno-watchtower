package kinds

import (
	"context"
	_ "embed"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/aeddi/gno-watchtower/internal/scribe/eventid"
	"github.com/aeddi/gno-watchtower/internal/scribe/handlers"
	sk "github.com/aeddi/gno-watchtower/internal/scribe/kinds"
	"github.com/aeddi/gno-watchtower/internal/scribe/normalizer"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

//go:embed validator_consensus_round_step.md
var validatorConsensusRoundStepDoc string

// rxRoundStep extracts the step name and height/round from enterXxx(h/r) messages.
var rxRoundStep = regexp.MustCompile(`enter(\w+)\((\d+)/(\d+)\)`)

// ConsensusRoundStep handles enterPropose/enterPrevote/enterPrecommit/enterCommit
// log lines and emits validator.consensus.round_step.
type ConsensusRoundStep struct{ cluster string }

// NewConsensusRoundStep returns a ConsensusRoundStep handler for the given cluster.
func NewConsensusRoundStep(cluster string) *ConsensusRoundStep {
	return &ConsensusRoundStep{cluster: cluster}
}

func (ConsensusRoundStep) Name() string { return "consensus_round_step" }

func (ConsensusRoundStep) Meta() handlers.Meta {
	return handlers.Meta{
		Kind:        "validator.consensus.round_step",
		Source:      handlers.SourceLog,
		Description: "Consensus state-machine step transition (enterPropose/Prevote/Precommit/Commit).",
		DocRef:      "/docs/handlers/validator.consensus.round_step",
	}
}

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
	payload := sk.ValidatorConsensusRoundStep{Height: height, Round: round, Step: step}
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

func init() {
	handlers.Register("validator.consensus.round_step",
		func(cluster string) handlers.Handler { return NewConsensusRoundStep(cluster) },
		validatorConsensusRoundStepDoc)
}
