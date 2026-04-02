package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

// runHealthServer starts a minimal HTTP server that responds 200 to GET /health.
// It blocks until ctx is done, then shuts down gracefully.
// If listenAddr is empty, it returns immediately.
func runHealthServer(ctx context.Context, listenAddr string, log *slog.Logger) {
	if listenAddr == "" {
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := &http.Server{Addr: listenAddr, Handler: mux}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			log.Error("health server shutdown", "err", err)
		}
	}()
	log.Info("health endpoint started", "addr", listenAddr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("health server error", "err", err)
	}
}
