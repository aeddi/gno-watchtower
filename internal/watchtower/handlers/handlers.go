package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/aeddi/gno-watchtower/internal/watchtower/auth"
	"github.com/aeddi/gno-watchtower/internal/watchtower/config"
	"github.com/aeddi/gno-watchtower/internal/watchtower/forwarder"
	"github.com/aeddi/gno-watchtower/internal/watchtower/ratelimit"
	"github.com/aeddi/gno-watchtower/internal/watchtower/stats"
)

// maxBodyBytes is the maximum request body size accepted by all endpoints.
const maxBodyBytes = 50 << 20 // 50 MB

// AuthCheckResponse is the JSON body of GET /auth/check.
type AuthCheckResponse struct {
	Validator    string   `json:"validator"`
	Permissions  []string `json:"permissions"`
	LogsMinLevel string   `json:"logs_min_level"`
}

// Server holds all dependencies and exposes the HTTP handler.
type Server struct {
	cfg   *config.Config
	auth  *auth.Authenticator
	rl    *ratelimit.Limiter
	fwd   *forwarder.Forwarder
	stats *stats.Stats
	log   *slog.Logger
}

// NewServer creates a Server.
func NewServer(
	cfg *config.Config,
	a *auth.Authenticator,
	rl *ratelimit.Limiter,
	fwd *forwarder.Forwarder,
	st *stats.Stats,
	log *slog.Logger,
) *Server {
	return &Server{cfg: cfg, auth: a, rl: rl, fwd: fwd, stats: st, log: log.With("component", "watchtower")}
}

// Handler returns the http.Handler with the full middleware chain.
// GET /health is unauthenticated (used by Docker healthcheck).
func (s *Server) Handler() http.Handler {
	inner := http.NewServeMux()
	inner.HandleFunc("POST /rpc", s.handleRPC)
	inner.HandleFunc("POST /metrics", s.handleMetrics)
	inner.HandleFunc("POST /logs", s.handleLogs)
	inner.HandleFunc("POST /otlp", s.handleOTLP)
	inner.HandleFunc("GET /auth/check", s.handleAuthCheck)

	outer := http.NewServeMux()
	outer.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	outer.Handle("/", s.auth.Middleware(s.rl.Middleware(inner)))
	return outer
}

// RunStatsLogger logs per-validator hourly stats on the given ticker until ctx is done.
func (s *Server) RunStatsLogger(ctx context.Context, ticker *time.Ticker) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			snap, uptime := s.stats.Snapshot()
			for validator, types := range snap {
				args := []any{"validator", validator, "uptime", uptime.Round(time.Second)}
				for typ, ts := range types {
					args = append(args, slog.Group(typ,
						"last_hour_bytes", ts.LastHourBytes,
						"total_bytes", ts.TotalBytes,
					))
				}
				s.log.Info("hourly stats", args...)
			}
		}
	}
}

func (s *Server) handlePayload(
	w http.ResponseWriter,
	r *http.Request,
	perm string,
	forward func(context.Context, string, []byte) error,
) {
	validator, vcfg, _ := auth.ValidatorFromContext(r.Context())
	if !slices.Contains(vcfg.Permissions, perm) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	s.log.Info("received", "validator", validator, "type", perm, "bytes", len(body))
	if err := forward(r.Context(), validator, body); err != nil {
		s.log.Error("forward "+perm, "err", err)
		http.Error(w, "forward failed", http.StatusBadGateway)
		return
	}
	s.stats.Record(validator, perm, len(body))
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleRPC(w http.ResponseWriter, r *http.Request) {
	s.handlePayload(w, r, "rpc", s.fwd.ForwardRPC)
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	s.handlePayload(w, r, "metrics", s.fwd.ForwardMetrics)
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	_, vcfg, _ := auth.ValidatorFromContext(r.Context())
	minLevel := vcfg.LogsMinLevel
	s.handlePayload(w, r, "logs", func(ctx context.Context, validator string, body []byte) error {
		return s.fwd.ForwardLogs(ctx, validator, body, minLevel)
	})
}

func (s *Server) handleOTLP(w http.ResponseWriter, r *http.Request) {
	s.handlePayload(w, r, "otlp", s.fwd.ForwardOTLP)
}

func (s *Server) handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	validator, vcfg, _ := auth.ValidatorFromContext(r.Context())
	resp := AuthCheckResponse{
		Validator:    validator,
		Permissions:  vcfg.Permissions,
		LogsMinLevel: vcfg.LogsMinLevel,
	}
	w.Header().Set("Content-Type", "application/json")
	// Encode after the Content-Type header but before the response body:
	// WriteHeader fires implicitly on the first Write, so we can't alter the
	// status if encoding fails mid-stream. Log-only is the honest option —
	// the client will see an incomplete body and retry.
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.log.Warn("auth check: encode response failed", "err", err)
	}
}
