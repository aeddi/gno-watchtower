package auth_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/watchtower/auth"
	"github.com/aeddi/gno-watchtower/internal/watchtower/config"
	wtmetrics "github.com/aeddi/gno-watchtower/internal/watchtower/metrics"
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
	// 403 distinguishes an auth-side ban (client misbehavior) from a 429
	// rate-limit signal (global back-pressure). Clients should not retry on 403.
	req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	req.Header.Set("Authorization", "Bearer good-token")
	req.RemoteAddr = "1.2.3.4:9999"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("want 403 for banned IP, got %d", rr.Code)
	}
}

func TestAuth_BanIsKeyedOnXForwardedForRightmost(t *testing.T) {
	// Behind a trusted proxy (Caddy), r.RemoteAddr is always the same
	// Docker-internal IP. The ban must segregate by the proxy-appended
	// XFF entry so one misbehaving validator doesn't poison the bucket
	// for every other validator on the same proxy.
	a := auth.New(map[string]config.ValidatorConfig{
		"val-01": {Token: "good-token"},
	}, 2, time.Minute) // ban after 2 failures

	handler := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	const proxyAddr = "172.18.0.2:52222" // stand-in for the Docker-internal Caddy IP

	// Two failures from client 10.0.0.1 via the proxy — should ban that client.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
		req.Header.Set("Authorization", "Bearer bad-token")
		req.Header.Set("X-Forwarded-For", "10.0.0.1")
		req.RemoteAddr = proxyAddr
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}

	// A different client via the same proxy must NOT be banned.
	reqOther := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	reqOther.Header.Set("Authorization", "Bearer good-token")
	reqOther.Header.Set("X-Forwarded-For", "10.0.0.2")
	reqOther.RemoteAddr = proxyAddr
	rrOther := httptest.NewRecorder()
	handler.ServeHTTP(rrOther, reqOther)
	if rrOther.Code != http.StatusOK {
		t.Fatalf("want 200 for uninvolved client via same proxy, got %d", rrOther.Code)
	}

	// The original client must still be banned.
	reqBanned := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	reqBanned.Header.Set("Authorization", "Bearer good-token")
	reqBanned.Header.Set("X-Forwarded-For", "10.0.0.1")
	reqBanned.RemoteAddr = proxyAddr
	rrBanned := httptest.NewRecorder()
	handler.ServeHTTP(rrBanned, reqBanned)
	if rrBanned.Code != http.StatusForbidden {
		t.Errorf("want 403 for banned client, got %d", rrBanned.Code)
	}
}

func TestAuth_XForwardedFor_TakesRightmostEntry(t *testing.T) {
	// Client-supplied XFF prefix could spoof arbitrary upstream IPs; the
	// proxy-appended rightmost entry is the authoritative client IP.
	a := auth.New(map[string]config.ValidatorConfig{
		"val-01": {Token: "good-token"},
	}, 2, time.Minute)

	handler := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Trip the real-client ban by sending 2 bad-token requests whose XFF
	// rightmost is 10.0.0.1.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
		req.Header.Set("Authorization", "Bearer bad-token")
		// Spoofed leftmost entries should be ignored.
		req.Header.Set("X-Forwarded-For", "192.0.2.99, 203.0.113.99, 10.0.0.1")
		req.RemoteAddr = "172.18.0.2:1234"
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}

	// Now a request from 10.0.0.1 (rightmost) should be banned even with a
	// valid token — proving we key on the rightmost entry.
	req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	req.Header.Set("Authorization", "Bearer good-token")
	req.Header.Set("X-Forwarded-For", "10.0.0.1")
	req.RemoteAddr = "172.18.0.2:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("want 403 keyed on rightmost XFF, got %d", rr.Code)
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

func TestAuth_SuccessfulAuthResetsFailures(t *testing.T) {
	a := auth.New(map[string]config.ValidatorConfig{
		"val-01": {Token: "good-token"},
	}, 3, time.Minute)

	handler := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Two failures — one short of the ban threshold.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
		req.Header.Set("Authorization", "Bearer bad-token")
		req.RemoteAddr = "1.2.3.4:9999"
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}

	// One success — must clear the failure counter.
	reqGood := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	reqGood.Header.Set("Authorization", "Bearer good-token")
	reqGood.RemoteAddr = "1.2.3.4:9999"
	handler.ServeHTTP(httptest.NewRecorder(), reqGood)

	// Two more failures would re-ban if the counter hadn't been reset.
	// With reset they leave us at failures=2 again, still unbanned.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
		req.Header.Set("Authorization", "Bearer bad-token")
		req.RemoteAddr = "1.2.3.4:9999"
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}

	reqCheck := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	reqCheck.Header.Set("Authorization", "Bearer good-token")
	reqCheck.RemoteAddr = "1.2.3.4:9999"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, reqCheck)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (still unbanned after reset), got %d", rr.Code)
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

func TestAuth_BannedCount_ReflectsActiveBans(t *testing.T) {
	// BannedCount must only count IPs currently under an active ban —
	// expired bans and non-banned failure records don't contribute. The
	// watchtower_banned_ips gauge hinges on this accuracy.
	a := auth.New(map[string]config.ValidatorConfig{
		"val-01": {Token: "good-token"},
	}, 1, 200*time.Millisecond) // ban on first failure, 200ms duration

	handler := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Empty ban set at start.
	if got := a.BannedCount(); got != 0 {
		t.Fatalf("BannedCount at start: got %d, want 0", got)
	}

	// Two different IPs both trigger a ban.
	for _, ip := range []string{"1.1.1.1", "2.2.2.2"} {
		req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
		req.Header.Set("Authorization", "Bearer bad-token")
		req.RemoteAddr = ip + ":1234"
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}
	if got := a.BannedCount(); got != 2 {
		t.Fatalf("BannedCount after 2 bans: got %d, want 2", got)
	}

	// After the ban duration expires, the count must drop.
	time.Sleep(250 * time.Millisecond)
	if got := a.BannedCount(); got != 0 {
		t.Errorf("BannedCount after expiry: got %d, want 0", got)
	}
}

func TestAuth_RecordsMetricsOnFailureAndBan(t *testing.T) {
	// Integration test: wire a real *metrics.Metrics into the Authenticator,
	// trip the ban (banThreshold=3) via three bad-token requests, send one more
	// from the now-banned IP, then scrape /metrics and assert both reason
	// variants. Using the real counters (not a spy) exercises the label values
	// the operator dashboard actually sees.
	a := auth.New(map[string]config.ValidatorConfig{
		"val-01": {Token: "good-token"},
	}, 3, time.Minute)
	m := wtmetrics.New()
	a.SetMetrics(m)

	handler := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Three bad-token requests from the same IP — all 401, ban arms on #3.
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
		req.Header.Set("Authorization", "Bearer bad-token")
		req.RemoteAddr = "9.9.9.9:1234"
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}

	// Fourth request from the banned IP — 403 banned, reason=banned.
	reqBanned := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	reqBanned.Header.Set("Authorization", "Bearer good-token")
	reqBanned.RemoteAddr = "9.9.9.9:1234"
	handler.ServeHTTP(httptest.NewRecorder(), reqBanned)

	// Scrape /metrics and check both label variants.
	srv := httptest.NewServer(m.Handler())
	defer srv.Close()
	resp, err := srv.Client().Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	text := string(body)

	wants := []string{
		`watchtower_auth_failures_total{reason="invalid_token"} 3`,
		`watchtower_auth_failures_total{reason="banned"} 1`,
	}
	for _, w := range wants {
		if !strings.Contains(text, w) {
			t.Errorf("missing line:\n  want: %s\n\nscrape:\n%s", w, text)
		}
	}
}
