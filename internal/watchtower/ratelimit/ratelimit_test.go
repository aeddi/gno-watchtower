package ratelimit_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gnolang/val-companion/internal/watchtower/auth"
	"github.com/gnolang/val-companion/internal/watchtower/config"
	"github.com/gnolang/val-companion/internal/watchtower/ratelimit"
)

// injectValidator returns a middleware that sets a validator in context.
func injectValidator(name string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cfg := config.ValidatorConfig{Token: "tok", Permissions: []string{"rpc"}, LogsMinLevel: "info"}
			ctx := auth.WithValidator(r.Context(), name, cfg)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func TestRateLimiter_AllowsUnderLimit(t *testing.T) {
	lim := ratelimit.New(100) // 100 rps — won't be hit in a single test request
	handler := injectValidator("val-01")(lim.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

func TestRateLimiter_BlocksOverLimit(t *testing.T) {
	lim := ratelimit.New(0.001) // ~1 request per 1000s — immediately exhausted

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := injectValidator("val-01")(lim.Middleware(inner))

	req := httptest.NewRequest(http.MethodPost, "/rpc", nil)

	// First request uses the single initial token.
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req)

	// Second request — no tokens left.
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req)

	if rr2.Code != http.StatusTooManyRequests {
		t.Errorf("want 429, got %d", rr2.Code)
	}
}

func TestRateLimiter_IndependentPerValidator(t *testing.T) {
	lim := ratelimit.New(0.001) // immediately exhausted
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Exhaust val-01.
	h1 := injectValidator("val-01")(lim.Middleware(inner))
	req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	h1.ServeHTTP(httptest.NewRecorder(), req)
	h1.ServeHTTP(httptest.NewRecorder(), req) // now exhausted

	// val-02 has its own bucket — should still pass.
	h2 := injectValidator("val-02")(lim.Middleware(inner))
	rr := httptest.NewRecorder()
	h2.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("val-02 want 200 (independent limiter), got %d", rr.Code)
	}
}
