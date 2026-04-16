package auth

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/aeddi/gno-watchtower/internal/watchtower/config"
)

// contextKey is the unexported type for context values set by this package.
type contextKey struct{}

type contextValue struct {
	name string
	cfg  config.ValidatorConfig
}

// ValidatorFromContext extracts the validator name and config set by Middleware.
func ValidatorFromContext(ctx context.Context) (string, config.ValidatorConfig, bool) {
	v, ok := ctx.Value(contextKey{}).(contextValue)
	return v.name, v.cfg, ok
}

// WithValidator sets the validator name and config into ctx.
// Used by tests and higher-level code to inject validator context.
func WithValidator(ctx context.Context, name string, cfg config.ValidatorConfig) context.Context {
	return context.WithValue(ctx, contextKey{}, contextValue{name: name, cfg: cfg})
}

type ipRecord struct {
	failures  int
	banExpiry time.Time
}

// Authenticator validates Bearer tokens and manages per-IP failure tracking.
type Authenticator struct {
	tokensMu     sync.RWMutex
	tokens       map[string]config.TokenEntry
	banThreshold int
	banDuration  time.Duration
	mu           sync.Mutex
	ips          map[string]*ipRecord
	lastCleanup  time.Time
}

// New creates an Authenticator from the token index in cfg.Validators.
func New(validators map[string]config.ValidatorConfig, banThreshold int, banDuration time.Duration) *Authenticator {
	tokens := make(map[string]config.TokenEntry, len(validators))
	for name, v := range validators {
		tokens[v.Token] = config.TokenEntry{ValidatorName: name, Config: v}
	}
	return &Authenticator{
		tokens:       tokens,
		banThreshold: banThreshold,
		banDuration:  banDuration,
		ips:          make(map[string]*ipRecord),
	}
}

// Reload atomically replaces the validator token map.
// Existing IP ban records are preserved.
func (a *Authenticator) Reload(validators map[string]config.ValidatorConfig) {
	tokens := make(map[string]config.TokenEntry, len(validators))
	for name, v := range validators {
		tokens[v.Token] = config.TokenEntry{ValidatorName: name, Config: v}
	}
	a.tokensMu.Lock()
	a.tokens = tokens
	a.tokensMu.Unlock()
}

// Middleware returns an http.Handler that enforces IP ban check and Bearer token auth.
// On success it sets the validator name and config in the request context.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := remoteIP(r)

		// Check IP ban before anything else.
		if a.isBanned(ip) {
			http.Error(w, "banned", http.StatusTooManyRequests)
			return
		}

		// Validate Bearer token.
		token := bearerToken(r)
		a.tokensMu.RLock()
		entry, ok := a.tokens[token]
		a.tokensMu.RUnlock()
		if !ok {
			a.recordFailure(ip)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), contextKey{}, contextValue{name: entry.ValidatorName, cfg: entry.Config})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *Authenticator) isBanned(ip string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.maybeSweep()
	rec, ok := a.ips[ip]
	if !ok {
		return false
	}
	if rec.banExpiry.IsZero() {
		return false
	}
	if time.Now().After(rec.banExpiry) {
		// Ban expired — reset.
		delete(a.ips, ip)
		return false
	}
	return true
}

// maybeSweep removes expired ban records if a full ban duration has elapsed since
// the last cleanup. Must be called with a.mu held.
func (a *Authenticator) maybeSweep() {
	if time.Since(a.lastCleanup) < a.banDuration {
		return
	}
	now := time.Now()
	for ip, rec := range a.ips {
		if !rec.banExpiry.IsZero() && now.After(rec.banExpiry) {
			delete(a.ips, ip)
		}
	}
	a.lastCleanup = now
}

func (a *Authenticator) recordFailure(ip string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	rec, ok := a.ips[ip]
	if !ok {
		rec = &ipRecord{}
		a.ips[ip] = rec
	}
	rec.failures++
	if rec.failures >= a.banThreshold {
		now := time.Now()
		rec.banExpiry = now.Add(a.banDuration)
	}
}

// bearerToken extracts the token from the Authorization: Bearer <token> header.
func bearerToken(r *http.Request) string {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return ""
	}
	return token
}

// remoteIP extracts the IP from r.RemoteAddr (host:port).
func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
