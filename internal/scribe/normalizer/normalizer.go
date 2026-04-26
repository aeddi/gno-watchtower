package normalizer

import (
	"context"

	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

const defaultLaneBuffer = 500

type Normalizer struct {
	out      chan<- types.Op
	handlers []Handler
	lanes    map[Lane]chan Observation
}

func New(out chan<- types.Op, handlers []Handler) *Normalizer {
	return &Normalizer{
		out:      out,
		handlers: handlers,
		lanes: map[Lane]chan Observation{
			LaneFast: make(chan Observation, defaultLaneBuffer),
			LaneSlow: make(chan Observation, defaultLaneBuffer),
			LaneLogs: make(chan Observation, defaultLaneBuffer),
		},
	}
}

// Input returns the producer-side channel for the named lane.
func (n *Normalizer) Input(l Lane) chan<- Observation {
	return n.lanes[l]
}

// Run consumes from all lanes (fast preferred) and dispatches to every handler.
func (n *Normalizer) Run(ctx context.Context) {
	for {
		// Strict priority: drain fast first.
		select {
		case <-ctx.Done():
			return
		case o := <-n.lanes[LaneFast]:
			n.dispatch(ctx, o)
		default:
			select {
			case <-ctx.Done():
				return
			case o := <-n.lanes[LaneFast]:
				n.dispatch(ctx, o)
			case o := <-n.lanes[LaneLogs]:
				n.dispatch(ctx, o)
			case o := <-n.lanes[LaneSlow]:
				n.dispatch(ctx, o)
			}
		}
	}
}

func (n *Normalizer) dispatch(ctx context.Context, o Observation) {
	for _, h := range n.handlers {
		for _, op := range h.Handle(ctx, o) {
			select {
			case n.out <- op:
			case <-ctx.Done():
				return
			}
		}
	}
}
