package handlers

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/normalizer"
	"github.com/aeddi/gno-watchtower/internal/scribe/sources/loki"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

func runLogHandler(t *testing.T, h normalizer.Handler, file string) []types.Op {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", file))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	o := normalizer.Observation{
		Lane: normalizer.LaneLogs, IngestTime: time.Now().UTC(),
		LogQuery: `{validator="node-1"}`,
		LogEntry: &loki.TailEntry{
			Stream: loki.Stream{Labels: map[string]string{"validator": "node-1", "module": "consensus"}},
			Time:   time.Now().UTC(),
			Line:   string(body),
		},
	}
	return h.Handle(context.Background(), o)
}

func TestProposedHandler(t *testing.T) {
	ops := runLogHandler(t, NewProposed("c1"), "validator.proposed.jsonl")
	if len(ops) != 1 || ops[0].Event.Kind != "validator.proposed" {
		t.Fatalf("got %+v", ops)
	}
}

func TestConsensusRoundStepHandler(t *testing.T) {
	ops := runLogHandler(t, NewConsensusRoundStep("c1"), "validator.consensus.round_step.jsonl")
	if len(ops) != 1 || ops[0].Event.Kind != "validator.consensus.round_step" {
		t.Fatalf("got %+v", ops)
	}
}

func TestVoteCastHandler(t *testing.T) {
	ops := runLogHandler(t, NewVoteCast("c1"), "validator.vote_cast.jsonl")
	if len(ops) != 1 || ops[0].Event.Kind != "validator.vote_cast" {
		t.Fatalf("got %+v", ops)
	}
}

func TestPeerConnectedHandler(t *testing.T) {
	ops := runLogHandler(t, NewPeerConnected("c1"), "validator.peer_connected.jsonl")
	if len(ops) != 1 || ops[0].Event.Kind != "validator.peer_connected" {
		t.Fatalf("got %+v", ops)
	}
}

func TestPeerDisconnectedHandler(t *testing.T) {
	ops := runLogHandler(t, NewPeerDisconnected("c1"), "validator.peer_disconnected.jsonl")
	if len(ops) != 1 || ops[0].Event.Kind != "validator.peer_disconnected" {
		t.Fatalf("got %+v", ops)
	}
}

func TestBlockCommittedHandler(t *testing.T) {
	ops := runLogHandler(t, NewBlockCommitted("c1"), "chain.block_committed.jsonl")
	if len(ops) != 1 || ops[0].Event.Kind != "chain.block_committed" {
		t.Fatalf("got %+v", ops)
	}
}

func TestValsetChangedHandler(t *testing.T) {
	ops := runLogHandler(t, NewValsetChanged("c1"), "chain.valset_changed.jsonl")
	if len(ops) != 1 || ops[0].Event.Kind != "chain.valset_changed" {
		t.Fatalf("got %+v", ops)
	}
}

func TestTxExecutedHandler(t *testing.T) {
	ops := runLogHandler(t, NewTxExecuted("c1"), "chain.tx_executed.jsonl")
	if len(ops) != 1 || ops[0].Event.Kind != "chain.tx_executed" {
		t.Fatalf("got %+v", ops)
	}
}
