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

func TestAuthenticator_Reload_UpdatesTokens(t *testing.T) {
	validators := map[string]config.ValidatorConfig{
		"val-01": {Token: "token-1", Permissions: []string{"rpc"}},
	}
	a := auth.New(validators, 5, time.Minute)

	// val-01 token works before reload.
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer token-1")
	w := httptest.NewRecorder()
	called := false
	a.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	})).ServeHTTP(w, req)
	if !called {
		t.Fatal("expected handler to be called for token-1 before reload")
	}

	// Reload with new validators — token-1 removed, token-2 added.
	a.Reload(map[string]config.ValidatorConfig{
		"val-02": {Token: "token-2", Permissions: []string{"metrics"}},
	})

	// token-1 must now be rejected.
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Header.Set("Authorization", "Bearer token-1")
	a.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("old token should be rejected after reload")
	})).ServeHTTP(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for old token, got %d", w2.Code)
	}

	// token-2 must now be accepted.
	w3 := httptest.NewRecorder()
	req3 := httptest.NewRequest("GET", "/", nil)
	req3.Header.Set("Authorization", "Bearer token-2")
	called3 := false
	a.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called3 = true
	})).ServeHTTP(w3, req3)
	if !called3 {
		t.Error("new token should be accepted after reload")
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
