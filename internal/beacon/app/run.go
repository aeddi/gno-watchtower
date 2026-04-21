// Package app wires config → Noise listener → augmenter → forwarder for the
// beacon. Run blocks until ctx is cancelled, then waits for the server
// goroutine to finish graceful shutdown before returning.
package app

import (
	"context"
	"log/slog"
	"sync"

	"github.com/aeddi/gno-watchtower/internal/beacon/augment"
	"github.com/aeddi/gno-watchtower/internal/beacon/config"
	"github.com/aeddi/gno-watchtower/internal/beacon/server"
)

// Run starts the Noise listener and HTTP forwarder, attaches the /rpc
// augmenter, and blocks until ctx is cancelled. A WaitGroup joins the server
// goroutine so main() can exit cleanly after Run returns.
func Run(ctx context.Context, cfg *config.Config, log *slog.Logger) {
	appLog := log.With("component", "app")

	noiseCfg, err := cfg.NoiseConfig()
	if err != nil {
		appLog.Error("load beacon keys", "err", err)
		return
	}

	augmenter := augment.New(cfg, nil, log)

	srv, err := server.New(server.Config{
		ListenAddr:       cfg.Beacon.ListenAddr,
		UpstreamURL:      cfg.Server.URL,
		NoiseConfig:      noiseCfg,
		HandshakeTimeout: cfg.Beacon.HandshakeTimeout.Duration,
		Transform:        augmenter.Transform,
		Log:              log,
	})
	if err != nil {
		appLog.Error("init server", "err", err)
		return
	}

	var wg sync.WaitGroup
	defer wg.Wait()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := srv.Run(ctx); err != nil && ctx.Err() == nil {
			appLog.Error("server stopped", "err", err)
		}
	}()

	<-ctx.Done()
	appLog.Info("beacon shutdown")
}
