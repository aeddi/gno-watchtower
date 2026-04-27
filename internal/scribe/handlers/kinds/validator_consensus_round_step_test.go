package kinds_test

import (
	"testing"

	"github.com/aeddi/gno-watchtower/internal/scribe/handlers/kinds"
)

func TestConsensusRoundStepHandler(t *testing.T) {
	ops := runLogHandler(t, kinds.NewConsensusRoundStep("c1"), "validator.consensus.round_step.jsonl")
	if len(ops) != 1 || ops[0].Event.Kind != "validator.consensus.round_step" {
		t.Fatalf("got %+v", ops)
	}
}
