package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"

	"github.com/aeddi/gno-watchtower/internal/watchtower/auth"
	"github.com/aeddi/gno-watchtower/internal/watchtower/config"
	"github.com/aeddi/gno-watchtower/internal/watchtower/forwarder"
	"github.com/aeddi/gno-watchtower/internal/watchtower/handlers"
	wtmetrics "github.com/aeddi/gno-watchtower/internal/watchtower/metrics"
	"github.com/aeddi/gno-watchtower/internal/watchtower/ratelimit"
	"github.com/aeddi/gno-watchtower/internal/watchtower/stats"
	"github.com/aeddi/gno-watchtower/pkg/logger"
)

func makeServer(t *testing.T, vmURL, lokiURL string) *handlers.Server {
	t.Helper()
	validators := map[string]config.ValidatorConfig{
		"val-01": {
			Token:        "test-token",
			Permissions:  []string{"rpc", "metrics", "logs", "otlp"},
			LogsMinLevel: "info",
		},
	}
	cfg := &config.Config{
		Security: config.SecurityConfig{
			RateLimitRPS: 100,
			BanThreshold: 10,
			BanDuration:  config.Duration{Duration: time.Minute},
		},
		Validators: validators,
	}
	a := auth.New(validators, cfg.Security.BanThreshold, cfg.Security.BanDuration.Duration)
	rl := ratelimit.New(cfg.Security.RateLimitRPS, 10, nil)
	fwd := forwarder.New(vmURL, lokiURL, nil)
	st := stats.New()
	return handlers.NewServer(cfg, a, rl, fwd, st, wtmetrics.New(), logger.Noop())
}

func authReq(method, path string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Authorization", "Bearer test-token")
	req.RemoteAddr = "127.0.0.1:9999"
	return req
}

func TestHandlerRPC_Returns200(t *testing.T) {
	vmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer vmSrv.Close()

	srv := makeServer(t, vmSrv.URL, "http://loki-unused:3100")
	body := []byte(`{"collected_at":"2026-01-01T00:00:00Z","data":{}}`)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, authReq(http.MethodPost, "/rpc", bytes.NewReader(body)))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandlerMetrics_Returns200(t *testing.T) {
	vmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer vmSrv.Close()

	srv := makeServer(t, vmSrv.URL, "http://loki-unused:3100")
	body := []byte(`{"collected_at":"2026-01-01T00:00:00Z","data":{}}`)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, authReq(http.MethodPost, "/metrics", bytes.NewReader(body)))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

func TestHandlerLogs_Returns200(t *testing.T) {
	lokiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer lokiSrv.Close()

	srv := makeServer(t, "http://vm-unused:8428", lokiSrv.URL)

	payload, _ := json.Marshal(struct {
		Level string            `json:"level"`
		Lines []json.RawMessage `json:"lines"`
	}{Level: "warn", Lines: []json.RawMessage{json.RawMessage(`{"level":"warn","msg":"hi"}`)}})

	var buf bytes.Buffer
	w, _ := zstd.NewWriter(&buf)
	w.Write(payload)
	w.Close()

	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, authReq(http.MethodPost, "/logs", &buf))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandlerAuthCheck_Returns200WithValidatorInfo(t *testing.T) {
	srv := makeServer(t, "http://vm:8428", "http://loki:3100")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, authReq(http.MethodGet, "/auth/check", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	var resp handlers.AuthCheckResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Validator != "val-01" {
		t.Errorf("validator: got %q", resp.Validator)
	}
	if resp.LogsMinLevel != "info" {
		t.Errorf("logs_min_level: got %q", resp.LogsMinLevel)
	}
}

func TestHandler_NoAuth_Returns401(t *testing.T) {
	srv := makeServer(t, "http://vm:8428", "http://loki:3100")
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader([]byte(`{}`)))
	req.RemoteAddr = "127.0.0.1:9999"
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestHandler_PermissionDenied_Returns403(t *testing.T) {
	// val-02 has only rpc+metrics permissions; trying /logs should return 403.
	validators := map[string]config.ValidatorConfig{
		"val-02": {
			Token:        "tok2",
			Permissions:  []string{"rpc", "metrics"},
			LogsMinLevel: "warn",
		},
	}
	cfg := &config.Config{
		Security: config.SecurityConfig{
			RateLimitRPS: 100,
			BanThreshold: 10,
			BanDuration:  config.Duration{Duration: time.Minute},
		},
		Validators: validators,
	}
	a := auth.New(validators, 10, time.Minute)
	rl := ratelimit.New(100, 10, nil)
	fwd := forwarder.New("http://vm:8428", "http://loki:3100", nil)
	srv := handlers.NewServer(cfg, a, rl, fwd, stats.New(), wtmetrics.New(), logger.Noop())

	req := httptest.NewRequest(http.MethodPost, "/logs", nil)
	req.Header.Set("Authorization", "Bearer tok2")
	req.RemoteAddr = "127.0.0.1:9999"
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rr.Code)
	}
}

func TestRunStatsLogger_LogsAndStops(t *testing.T) {
	srv := makeServer(t, "http://vm:8428", "http://loki:3100")
	ctx, cancel := context.WithCancel(context.Background())

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	done := make(chan struct{})
	go func() {
		srv.RunStatsLogger(ctx, ticker)
		close(done)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Error("RunStatsLogger did not stop after context cancel")
	}
}

func TestHandler_Health_Returns200WithoutAuth(t *testing.T) {
	srv := makeServer(t, "http://vm:8428", "http://loki:3100")
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.RemoteAddr = "127.0.0.1:9999"
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}
