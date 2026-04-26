package normalizer

import (
	"context"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

type recordingHandler struct {
	name string
	got  []Observation
}

func (h *recordingHandler) Name() string { return h.name }
func (h *recordingHandler) Handle(_ context.Context, o Observation) []types.Op {
	h.got = append(h.got, o)
	return nil
}

type opEmittingHandler struct{}

func (opEmittingHandler) Name() string { return "op-emitter" }
func (opEmittingHandler) Handle(_ context.Context, o Observation) []types.Op {
	ev := &types.Event{Kind: "x", Subject: "node-1"}
	return []types.Op{{Kind: types.OpInsertEvent, Event: ev, FromBackfill: o.Lane == LaneSlow}}
}

func TestNormalizerDispatchesToAllHandlers(t *testing.T) {
	h1 := &recordingHandler{name: "h1"}
	h2 := &recordingHandler{name: "h2"}
	out := make(chan types.Op, 4)
	n := New(out, []Handler{h1, h2})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go n.Run(ctx)

	in := n.Input(LaneFast)
	in <- Observation{Lane: LaneFast, IngestTime: time.Now()}

	deadline := time.After(time.Second)
	for h1.got == nil || h2.got == nil {
		select {
		case <-deadline:
			t.Fatalf("not all handlers received: h1=%v h2=%v", h1.got, h2.got)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestNormalizerForwardsOps(t *testing.T) {
	out := make(chan types.Op, 4)
	n := New(out, []Handler{opEmittingHandler{}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go n.Run(ctx)

	n.Input(LaneFast) <- Observation{Lane: LaneFast, IngestTime: time.Now()}
	select {
	case op := <-out:
		if op.Event == nil || op.Event.Kind != "x" {
			t.Errorf("got %+v", op)
		}
		if op.FromBackfill {
			t.Error("LaneFast must not produce backfill ops")
		}
	case <-time.After(time.Second):
		t.Fatal("no op forwarded")
	}
}
