package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/projection"
)

func (s *Server) handleStateImpl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method_not_allowed", "", "")
		return
	}
	q := r.URL.Query()
	at := time.Now().UTC()
	if v := q.Get("at"); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			writeError(w, 400, "bad_at", err.Error(), "")
			return
		}
		at = t
	}

	if subs := q.Get("subjects"); subs != "" {
		subjects := strings.Split(subs, ",")
		states := map[string]map[string]any{}
		for _, sub := range subjects {
			sub = strings.TrimSpace(sub)
			st, replayed, err := projection.ProjectStateAt(r.Context(), s.deps.Store, s.deps.ClusterID, sub, at)
			if err != nil {
				writeError(w, 500, "store_error", err.Error(), "")
				return
			}
			fastScalars := map[string]any{}
			if sub == "_chain" {
				if c, _ := s.deps.Store.GetLatestSampleChain(r.Context(), s.deps.ClusterID, at); c != nil {
					fastScalars["block_height"] = c.BlockHeight
					fastScalars["online_count"] = c.OnlineCount
					fastScalars["valset_size"] = c.ValsetSize
				}
			} else {
				// Merge across the per-handler rows so each column has its real
				// value (handlers other than the column's owner write zeros).
				if v, _ := s.deps.Store.GetMergedSampleValidator(r.Context(), s.deps.ClusterID, sub, at, 30*time.Second); v != nil {
					fastScalars["height"] = v.Height
					fastScalars["voting_power"] = v.VotingPower
					fastScalars["catching_up"] = v.CatchingUp
					fastScalars["mempool_txs"] = v.MempoolTxs
					fastScalars["cpu_pct"] = v.CPUPct
					fastScalars["mem_pct"] = v.MemPct
					if v.BehindSentry != nil {
						fastScalars["behind_sentry"] = *v.BehindSentry
					}
				}
			}
			states[sub] = map[string]any{
				"fast_scalars":    fastScalars,
				"structured":      st,
				"events_replayed": replayed,
			}
		}
		writeJSON(w, 200, map[string]any{"at": at, "states": states})
		return
	}

	sub := q.Get("subject")
	if sub == "" {
		writeError(w, 400, "missing_subject", "either subject or subjects must be provided", "")
		return
	}
	st, replayed, err := projection.ProjectStateAt(r.Context(), s.deps.Store, s.deps.ClusterID, sub, at)
	if err != nil {
		writeError(w, 500, "store_error", err.Error(), "")
		return
	}
	fastScalars := map[string]any{}
	if sub == "_chain" {
		if c, _ := s.deps.Store.GetLatestSampleChain(r.Context(), s.deps.ClusterID, at); c != nil {
			fastScalars["block_height"] = c.BlockHeight
			fastScalars["online_count"] = c.OnlineCount
			fastScalars["valset_size"] = c.ValsetSize
		}
	} else {
		if v, _ := s.deps.Store.GetMergedSampleValidator(r.Context(), s.deps.ClusterID, sub, at, 30*time.Second); v != nil {
			fastScalars["height"] = v.Height
			fastScalars["voting_power"] = v.VotingPower
			fastScalars["catching_up"] = v.CatchingUp
			fastScalars["mempool_txs"] = v.MempoolTxs
			fastScalars["cpu_pct"] = v.CPUPct
			fastScalars["mem_pct"] = v.MemPct
			if v.BehindSentry != nil {
				fastScalars["behind_sentry"] = *v.BehindSentry
			}
		}
	}
	writeJSON(w, 200, map[string]any{
		"subject": sub, "at": at,
		"fast_scalars":    fastScalars,
		"structured":      st,
		"events_replayed": replayed,
	})
}
