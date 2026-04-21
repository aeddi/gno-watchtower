// Package app wires config → Noise listener → forwarder for the beacon.
// Run blocks until ctx is cancelled, then performs graceful shutdown.
package app

import (
	"context"
	"log/slog"

	"github.com/aeddi/gno-watchtower/internal/beacon/config"
)

// Run is a stub in this commit. Actual listener + forwarder wiring lands in
// C4; this function exists so cmd/beacon compiles and the CLI contract is
// stable from C1 onward.
func Run(ctx context.Context, cfg *config.Config, log *slog.Logger) {
	log.Info("beacon stub running — full wiring lands in a later commit",
		"upstream", cfg.Server.URL,
		"listen", cfg.Beacon.ListenAddr,
	)
	<-ctx.Done()
	log.Info("beacon shutdown")
}
