package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gnolang/val-companion/internal/watchtower/auth"
	"github.com/gnolang/val-companion/internal/watchtower/config"
)

func makeAuth(t *testing.T) *auth.Authenticator {
	t.Helper()
	validators := map[string]config.ValidatorConfig{
		"val-01": {
			Token:        "good-token",
			Permissions:  []string{"rpc", "metrics", "logs", "otlp"},
			LogsMinLevel: "info",
		},
	}
	return auth.New(validators, 3, time.Minute)
}

func TestAuth_ValidToken_Sets200AndValidatorInContext(t *testing.T) {
	a := makeAuth(t)

	called := false
	handler := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		name, cfg, ok := auth.ValidatorFromContext(r.Context())
		if !ok {
			t.Error("validator not in context")
		}
		if name != "val-01" {
			t.Errorf("validator name: got %q", name)
		}
		if cfg.Token != "good-token" {
			t.Errorf("validator config token: got %q", cfg.Token)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	req.Header.Set("Authorization", "Bearer good-token")
	req.RemoteAddr = "1.2.3.4:9999"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("handler not called")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

func TestAuth_InvalidToken_Returns401(t *testing.T) {
	a := makeAuth(t)
	handler := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler must not be called on 401")
	}))

	req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	req.RemoteAddr = "1.2.3.4:9999"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestAuth_BansIPAfterThreshold(t *testing.T) {
	a := auth.New(map[string]config.ValidatorConfig{
		"val-01": {Token: "good-token"},
	}, 2, time.Minute) // ban after 2 failures

	handler := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	// Two failures from the same IP.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
		req.Header.Set("Authorization", "Bearer bad-token")
		req.RemoteAddr = "1.2.3.4:9999"
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}

	// Third request — IP should now be banned (even with a valid token).
	req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	req.Header.Set("Authorization", "Bearer good-token")
	req.RemoteAddr = "1.2.3.4:9999"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("want 429 for banned IP, got %d", rr.Code)
	}
}

func TestAuth_BanExpires(t *testing.T) {
	a := auth.New(map[string]config.ValidatorConfig{
		"val-01": {Token: "good-token"},
	}, 1, 10*time.Millisecond) // very short ban

	handler := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Trigger ban.
	req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	req.Header.Set("Authorization", "Bearer bad-token")
	req.RemoteAddr = "1.2.3.4:9999"
	handler.ServeHTTP(httptest.NewRecorder(), req)

	// Wait for ban to expire.
	time.Sleep(20 * time.Millisecond)

	// Should succeed now.
	req2 := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	req2.Header.Set("Authorization", "Bearer good-token")
	req2.RemoteAddr = "1.2.3.4:9999"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req2)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200 after ban expires, got %d", rr.Code)
	}
}

func TestAuth_MissingAuthHeader_Returns401(t *testing.T) {
	a := makeAuth(t)
	handler := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler must not be called")
	}))

	req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	req.RemoteAddr = "1.2.3.4:9999"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}
