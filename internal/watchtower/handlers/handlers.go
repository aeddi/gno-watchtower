package handlers

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gnolang/val-companion/internal/watchtower/auth"
	"github.com/gnolang/val-companion/internal/watchtower/config"
	"github.com/gnolang/val-companion/internal/watchtower/forwarder"
	"github.com/gnolang/val-companion/internal/watchtower/ratelimit"
	"github.com/gnolang/val-companion/internal/watchtower/stats"
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
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rpc", s.handleRPC)
	mux.HandleFunc("POST /metrics", s.handleMetrics)
	mux.HandleFunc("POST /logs", s.handleLogs)
	mux.HandleFunc("POST /otlp", s.handleOTLP)
	mux.HandleFunc("GET /auth/check", s.handleAuthCheck)
	return s.auth.Middleware(s.rl.Middleware(mux))
}

// RunStatsLogger logs per-validator hourly stats on the given ticker until ctx is done.
func (s *Server) RunStatsLogger(ctx interface{ Done() <-chan struct{} }, ticker *time.Ticker) {
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

func (s *Server) handleRPC(w http.ResponseWriter, r *http.Request) {
	validator, vcfg, _ := auth.ValidatorFromContext(r.Context())
	if !hasPermission(vcfg.Permissions, "rpc") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	s.log.Info("received", "validator", validator, "type", "rpc", "bytes", len(body))
	if err := s.fwd.ForwardRPC(r.Context(), validator, body); err != nil {
		s.log.Error("forward rpc", "err", err)
		http.Error(w, "forward failed", http.StatusBadGateway)
		return
	}
	s.stats.Record(validator, "rpc", len(body))
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	validator, vcfg, _ := auth.ValidatorFromContext(r.Context())
	if !hasPermission(vcfg.Permissions, "metrics") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	s.log.Info("received", "validator", validator, "type", "metrics", "bytes", len(body))
	if err := s.fwd.ForwardMetrics(r.Context(), validator, body); err != nil {
		s.log.Error("forward metrics", "err", err)
		http.Error(w, "forward failed", http.StatusBadGateway)
		return
	}
	s.stats.Record(validator, "metrics", len(body))
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	validator, vcfg, _ := auth.ValidatorFromContext(r.Context())
	if !hasPermission(vcfg.Permissions, "logs") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	s.log.Info("received", "validator", validator, "type", "logs", "bytes", len(body))
	if err := s.fwd.ForwardLogs(r.Context(), validator, body); err != nil {
		s.log.Error("forward logs", "err", err)
		http.Error(w, "forward failed", http.StatusBadGateway)
		return
	}
	s.stats.Record(validator, "logs", len(body))
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleOTLP(w http.ResponseWriter, r *http.Request) {
	validator, vcfg, _ := auth.ValidatorFromContext(r.Context())
	if !hasPermission(vcfg.Permissions, "otlp") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	s.log.Info("received", "validator", validator, "type", "otlp", "bytes", len(body))
	if err := s.fwd.ForwardOTLP(r.Context(), validator, body); err != nil {
		s.log.Error("forward otlp", "err", err)
		http.Error(w, "forward failed", http.StatusBadGateway)
		return
	}
	s.stats.Record(validator, "otlp", len(body))
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	validator, vcfg, _ := auth.ValidatorFromContext(r.Context())
	resp := AuthCheckResponse{
		Validator:    validator,
		Permissions:  vcfg.Permissions,
		LogsMinLevel: vcfg.LogsMinLevel,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

func hasPermission(permissions []string, perm string) bool {
	for _, p := range permissions {
		if p == perm {
			return true
		}
	}
	return false
}
