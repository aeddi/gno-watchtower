package doctor

import (
	"context"
	"fmt"
	"io"

	"github.com/aeddi/gno-watchtower/internal/beacon/config"
)

// Run executes all beacon doctor checks, printing each result as it completes.
// Exit code 0 = no Red results. Exit code 1 = at least one Red.
func Run(ctx context.Context, cfg *config.Config, configPath string, w io.Writer) int {
	fmt.Fprintf(w, "Validating beacon config: %s\n", configPath)
	hasRed := false

	emit := func(r CheckResult) {
		fmt.Fprintln(w, formatResult(r))
		if r.Status == StatusRed {
			hasRed = true
		}
	}

	emit(CheckWatchtower(ctx, cfg.Server.URL))
	emit(CheckKeypair(cfg.Beacon))
	emit(CheckAuthorizedKeys(cfg.Beacon))
	emit(CheckRPC(ctx, cfg.RPC.RPCURL))
	emit(CheckMetadataConfig(ctx, cfg.Metadata))

	fmt.Fprintln(w)
	if hasRed {
		return 1
	}
	return 0
}
