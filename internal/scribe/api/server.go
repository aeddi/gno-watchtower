// Package api implements the read-only HTTP API for the scribe service.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/aeddi/gno-watchtower/internal/scribe/cache"
	"github.com/aeddi/gno-watchtower/internal/scribe/store"
	"github.com/aeddi/gno-watchtower/internal/scribe/writer"
)

// Deps groups dependencies the API handlers need.
type Deps struct {
	Store     store.Store
	Cache     *cache.Cache
	Writer    *writer.Writer
	ClusterID string
	// Metrics is added in Phase 11; keep the field optional for now.
}

// Server wraps the read-only HTTP API.
type Server struct {
	deps Deps
}

// New returns a new Server with the given dependencies.
func New(deps Deps) *Server { return &Server{deps: deps} }

// Handler exposes the underlying mux for embedding into other servers (used by
// cmd/scribe runCmdImpl tests and the production main entrypoint).
func (s *Server) Handler() http.Handler { return s.http() }

func (s *Server) http() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/events", s.handleEventsImpl)
	mux.HandleFunc("/api/events/stream", s.handleEventsStream)
	mux.HandleFunc("/api/state", s.handleStateImpl)
	mux.HandleFunc("/api/samples", s.handleSamples)
	mux.HandleFunc("/api/subjects", s.handleSubjects)
	mux.HandleFunc("/api/event-kinds", s.handleEventKinds)
	mux.HandleFunc("/api/backfill", s.handleBackfill)
	mux.HandleFunc("/api/backfill/", s.handleBackfillID)
	return mux
}

// ListenAndServe runs the HTTP server until the underlying http.Server is shut down.
func (s *Server) ListenAndServe(addr string) error {
	srv := &http.Server{Addr: addr, Handler: s.http()}
	return srv.ListenAndServe()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, detail, requestID string) {
	writeJSON(w, status, map[string]any{
		"error": code, "detail": detail, "request_id": requestID,
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// All other handlers below return 501 for now; subsequent Phase-8 tasks fill them in.
func (s *Server) handleEventsStream(w http.ResponseWriter, _ *http.Request) {
	writeError(w, 501, "not_implemented", "", "")
}

func (s *Server) handleSamples(w http.ResponseWriter, _ *http.Request) {
	writeError(w, 501, "not_implemented", "", "")
}

func (s *Server) handleSubjects(w http.ResponseWriter, _ *http.Request) {
	writeError(w, 501, "not_implemented", "", "")
}

func (s *Server) handleEventKinds(w http.ResponseWriter, _ *http.Request) {
	writeError(w, 501, "not_implemented", "", "")
}

func (s *Server) handleBackfill(w http.ResponseWriter, _ *http.Request) {
	writeError(w, 501, "not_implemented", "", "")
}

func (s *Server) handleBackfillID(w http.ResponseWriter, _ *http.Request) {
	writeError(w, 501, "not_implemented", "", "")
}
