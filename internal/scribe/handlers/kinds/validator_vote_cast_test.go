package kinds_test

import (
	"testing"

	"github.com/aeddi/gno-watchtower/internal/scribe/handlers/kinds"
)

func TestVoteCastHandler(t *testing.T) {
	ops := runLogHandler(t, kinds.NewVoteCast("c1"), "validator.vote_cast.jsonl")
	if len(ops) != 1 || ops[0].Event.Kind != "validator.vote_cast" {
		t.Fatalf("got %+v", ops)
	}
}
