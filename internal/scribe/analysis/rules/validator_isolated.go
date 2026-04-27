package rules

import (
	"context"
	_ "embed"
	"sync"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/analysis"
	"github.com/aeddi/gno-watchtower/internal/scribe/eventid"
)

//go:embed validator_isolated.md
var validatorIsolatedDoc string

// ValidatorIsolatedRule fires when a validator's combined peer count has
// been below min_peers for at least isolated_threshold_seconds. Sustained
// — a single transient flap doesn't trigger. Subscribes to peer events AND
// a 30s tick so a node staying low-peer without new peer events still gets
// caught.
type ValidatorIsolatedRule struct {
	tracker analysis.Tracker

	mu             sync.Mutex
	firstLowSeenAt map[string]time.Time // key (cluster|validator) -> when peer count first dipped below min_peers
}

// Meta returns the rule descriptor used by the engine and /api/rules.
func (r *ValidatorIsolatedRule) Meta() analysis.Meta {
	return analysis.Meta{
		Code:        "validator_isolated",
		Version:     1,
		Severity:    analysis.SeverityWarning,
		Kinds:       []string{"validator.peer_connected", "validator.peer_disconnected"},
		TickPeriod:  30 * time.Second,
		Description: "Validator peer count below min_peers for a sustained interval.",
		Params: map[string]analysis.ParamSpec{
			"min_peers":                  {Default: int64(2), Min: int64(1), Max: int64(100)},
			"isolated_threshold_seconds": {Default: int64(30), Min: int64(5), Max: int64(3600)},
		},
	}
}

// RecoveryTracker exposes the rule's recovery state to the engine for rehydration.
func (r *ValidatorIsolatedRule) RecoveryTracker() *analysis.Tracker { return &r.tracker }

// Evaluate enumerates known validator subjects (filtered to the trigger
// subject when event-driven), reads each one's latest merged samples, and
// transitions per-validator open/recovered state based on sustained low
// peer count.
func (r *ValidatorIsolatedRule) Evaluate(ctx context.Context, t analysis.Trigger, d analysis.Deps, emit analysis.Emitter) {
	subjects, err := d.Store.ListSubjects(ctx, d.ClusterID)
	if err != nil {
		return
	}
	now := d.Now()
	minPeers := int16(d.Config.Int("min_peers"))
	isoThreshold := time.Duration(d.Config.Int("isolated_threshold_seconds")) * time.Second

	r.mu.Lock()
	if r.firstLowSeenAt == nil {
		r.firstLowSeenAt = map[string]time.Time{}
	}
	r.mu.Unlock()

	for _, sub := range subjects {
		if sub == "_chain" {
			continue
		}
		// On event-driven triggers, only consider the subject the event is about.
		if t.Event != nil && t.Event.Subject != sub {
			continue
		}
		sample, err := d.Store.GetMergedSampleValidator(ctx, d.ClusterID, sub, now, 30*time.Second)
		if err != nil || sample == nil {
			continue
		}
		peers := sample.PeerCountIn + sample.PeerCountOut
		key := d.ClusterID + "|" + sub

		if peers < minPeers {
			r.mu.Lock()
			first, seen := r.firstLowSeenAt[key]
			if !seen {
				r.firstLowSeenAt[key] = now
				r.mu.Unlock()
				continue
			}
			r.mu.Unlock()
			if now.Sub(first) < isoThreshold {
				continue
			}
			eid := eventid.Derive(now, r.Meta().Kind(), key, []byte(key+"open"))
			if r.tracker.Open(key, eid) {
				emit(analysis.Diagnostic{
					Subject: sub,
					State:   analysis.StateOpen,
					Payload: map[string]any{
						"recovery_key":   key,
						"peer_count_in":  sample.PeerCountIn,
						"peer_count_out": sample.PeerCountOut,
						"min_peers":      int64(minPeers),
						"low_since":      first,
					},
				})
			}
			continue
		}
		// Peers have returned. Clear sustained-low timer and recover any open incident.
		r.mu.Lock()
		delete(r.firstLowSeenAt, key)
		r.mu.Unlock()
		if openID := r.tracker.Recovered(key); openID != "" {
			emit(analysis.Diagnostic{
				Subject:  sub,
				State:    analysis.StateRecovered,
				Recovers: openID,
				Payload: map[string]any{
					"recovery_key":   key,
					"peer_count_in":  sample.PeerCountIn,
					"peer_count_out": sample.PeerCountOut,
				},
			})
		}
	}
}

func init() {
	analysis.Register(&ValidatorIsolatedRule{}, validatorIsolatedDoc)
}
