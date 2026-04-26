package api

import (
	"net/http"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/store"
)

func (s *Server) handleSamplesImpl(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, err := parseTime(q.Get("from"))
	if err != nil || from.IsZero() {
		writeError(w, 400, "bad_from", "from is required", "")
		return
	}
	to, err := parseTime(q.Get("to"))
	if err != nil || to.IsZero() {
		writeError(w, 400, "bad_to", "to is required", "")
		return
	}
	step, err := time.ParseDuration(q.Get("step"))
	if err != nil || step <= 0 {
		step = time.Minute
	}
	if int64(to.Sub(from)/step) > 1000 {
		suggested := time.Duration(int64(to.Sub(from)) / 1000)
		writeError(w, 400, "invalid_step",
			"step would yield >1000 buckets in window; suggest "+suggested.String(), "")
		return
	}
	subject := q.Get("subject")
	if subject == "" {
		writeError(w, 400, "missing_subject", "subject required", "")
		return
	}
	if subject == "_chain" {
		buckets, err := s.deps.Store.BucketChainSamples(r.Context(), store.SamplesQuery{
			ClusterID: s.deps.ClusterID, From: from, To: to, Step: step,
		})
		if err != nil {
			writeError(w, 500, "store_error", err.Error(), "")
			return
		}
		writeJSON(w, 200, map[string]any{"subject": subject, "step": step.String(), "from": from, "to": to, "buckets": buckets})
		return
	}
	buckets, err := s.deps.Store.BucketValidatorSamples(r.Context(), store.SamplesQuery{
		ClusterID: s.deps.ClusterID, Subject: subject, From: from, To: to, Step: step,
	})
	if err != nil {
		writeError(w, 500, "store_error", err.Error(), "")
		return
	}
	writeJSON(w, 200, map[string]any{"subject": subject, "step": step.String(), "from": from, "to": to, "buckets": buckets})
}
