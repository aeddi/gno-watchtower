package projection

import (
	"context"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/store"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

// ProjectStateAt returns the projected structured state for (cluster, subject) at `at`.
// Returns the state map, the number of events replayed (for observability), and error.
func ProjectStateAt(ctx context.Context, s store.Store, cluster, subject string, at time.Time) (map[string]any, int, error) {
	anchor, err := s.GetLatestAnchor(ctx, cluster, subject, at)
	if err != nil {
		return nil, 0, err
	}
	state := map[string]any{}
	if anchor != nil {
		for k, v := range anchor.FullState {
			state[k] = v
		}
	}

	// Replay events from anchor.events_through to `at`.
	cursor := ""
	if anchor != nil {
		cursor = anchor.EventsThrough
	}
	replayed := 0
	for {
		evs, next, err := s.QueryEvents(ctx, store.EventQuery{
			ClusterID: cluster, Subject: subject, From: time.Time{}, To: at,
			Limit: 1000, Cursor: cursor,
		})
		if err != nil {
			return nil, replayed, err
		}
		for _, e := range evs {
			apply(state, e)
			replayed++
		}
		if next == "" {
			break
		}
		cursor = next
	}
	return state, replayed, nil
}

// apply mutates `state` to reflect event `e`. Each event kind has its own projection
// rule. Unknown kinds are no-ops (forward-compat with future kinds).
func apply(state map[string]any, e types.Event) {
	switch e.Kind {
	case "validator.peer_connected":
		peers, _ := state["peers"].(map[string]any)
		if peers == nil {
			peers = map[string]any{}
			state["peers"] = peers
		}
		if id, ok := e.Payload["peer_id"].(string); ok && id != "" {
			peers[id] = e.Payload
		}
	case "validator.peer_disconnected":
		if peers, ok := state["peers"].(map[string]any); ok {
			if id, ok := e.Payload["peer_id"].(string); ok {
				delete(peers, id)
			}
		}
	case "validator.config_changed":
		state["config_hash"] = e.Payload["new"]
	case "validator.consensus.round_step":
		locks, _ := state["consensus_locks"].(map[string]any)
		if locks == nil {
			locks = map[string]any{}
			state["consensus_locks"] = locks
		}
		locks["height"] = e.Payload["height"]
		locks["round"] = e.Payload["round"]
		locks["step"] = e.Payload["step"]
	case "chain.valset_changed":
		// Simplified: future iteration will apply adds/removes incrementally.
		state["valset_view"] = e.Payload["added"]
	default:
		// no-op
	}
}
