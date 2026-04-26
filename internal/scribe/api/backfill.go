package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/store"
)

func newID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (s *Server) handleBackfillImpl(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req struct {
			From      time.Time `json:"from"`
			To        time.Time `json:"to"`
			ChunkSize string    `json:"chunk_size"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, 400, "bad_body", err.Error(), "")
			return
		}
		if req.To.IsZero() {
			req.To = time.Now().Add(-5 * time.Minute)
		}
		if req.From.IsZero() {
			req.From = time.Now().Add(-30 * 24 * time.Hour)
		}
		if !req.From.Before(req.To) {
			writeError(w, 400, "bad_range", "from must be before to", "")
			return
		}
		chunk := time.Hour
		if req.ChunkSize != "" {
			d, err := time.ParseDuration(req.ChunkSize)
			if err != nil {
				writeError(w, 400, "bad_chunk_size", err.Error(), "")
				return
			}
			chunk = d
		}
		j := store.BackfillJob{
			ID: newID(), ClusterID: s.deps.ClusterID,
			From: req.From, To: req.To, ChunkSize: chunk,
			StartedAt: time.Now(), LastProgressAt: time.Now(),
			Status: "pending",
		}
		if err := s.deps.Store.UpsertBackfillJob(r.Context(), j); err != nil {
			writeError(w, 500, "store_error", err.Error(), "")
			return
		}
		writeJSON(w, 202, j)
		return
	case http.MethodGet:
		jobs, err := s.deps.Store.ListBackfillJobs(r.Context(), s.deps.ClusterID, 50)
		if err != nil {
			writeError(w, 500, "store_error", err.Error(), "")
			return
		}
		writeJSON(w, 200, map[string]any{"jobs": jobs})
		return
	default:
		writeError(w, 405, "method_not_allowed", "", "")
	}
}

func (s *Server) handleBackfillIDImpl(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/backfill/")
	if id == "" {
		writeError(w, 400, "missing_id", "", "")
		return
	}
	switch r.Method {
	case http.MethodGet:
		j, err := s.deps.Store.GetBackfillJob(r.Context(), id)
		if err != nil {
			writeError(w, 500, "store_error", err.Error(), "")
			return
		}
		if j == nil {
			writeError(w, 404, "not_found", "", "")
			return
		}
		writeJSON(w, 200, j)
	case http.MethodDelete:
		j, err := s.deps.Store.GetBackfillJob(r.Context(), id)
		if err != nil {
			writeError(w, 500, "store_error", err.Error(), "")
			return
		}
		if j == nil {
			writeError(w, 404, "not_found", "", "")
			return
		}
		j.Status = "cancelled"
		j.LastProgressAt = time.Now()
		if err := s.deps.Store.UpsertBackfillJob(r.Context(), *j); err != nil {
			writeError(w, 500, "store_error", err.Error(), "")
			return
		}
		writeJSON(w, 200, j)
	default:
		writeError(w, 405, "method_not_allowed", "", "")
	}
}
