package ratelimit

import (
	"net/http"
	"sync"

	"golang.org/x/time/rate"

	"github.com/gnolang/val-companion/internal/watchtower/auth"
)

// Limiter enforces a per-validator token-bucket rate limit.
type Limiter struct {
	rps      rate.Limit
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
}

// New creates a Limiter with the given requests-per-second rate.
func New(rps float64) *Limiter {
	return &Limiter{
		rps:      rate.Limit(rps),
		limiters: make(map[string]*rate.Limiter),
	}
}

// Middleware returns an http.Handler that enforces per-validator rate limiting.
// It requires auth.Middleware to run first (to set the validator in context).
func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name, _, ok := auth.ValidatorFromContext(r.Context())
		if !ok {
			// No validator in context — auth middleware should have rejected this.
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !l.allow(name) {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (l *Limiter) allow(validatorName string) bool {
	l.mu.Lock()
	lim, ok := l.limiters[validatorName]
	if !ok {
		lim = rate.NewLimiter(l.rps, 1)
		l.limiters[validatorName] = lim
	}
	l.mu.Unlock()
	return lim.Allow()
}
