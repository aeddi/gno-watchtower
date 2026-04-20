// internal/sentinel/doctor/run.go
package doctor

import (
	"context"
	"fmt"
	"io"
	"slices"

	"github.com/aeddi/gno-watchtower/internal/sentinel/config"
	"github.com/aeddi/gno-watchtower/internal/sentinel/logs"
	pkglogger "github.com/aeddi/gno-watchtower/pkg/logger"
)

// Run executes all doctor checks, printing each result as it completes.
// Exit code 0 = no Red results. Exit code 1 = at least one Red result.
func Run(ctx context.Context, cfg *config.Config, configPath string, w io.Writer) int {
	fmt.Fprintf(w, "Validating sentinel config: %s\n", configPath)
	hasRed := false

	emit := func(results ...CheckResult) {
		for _, r := range results {
			fmt.Fprintln(w, formatResult(r))
			if r.Status == StatusRed {
				hasRed = true
			}
		}
	}

	// ---- Remote + token + permissions (always run; drives Grey status below)
	remoteResults, authResp := CheckRemoteTokenAndPermissions(ctx, cfg)
	emit(remoteResults...)

	// ---- Metadata checks
	emit(metadataChecks(cfg, authResp)...)

	// ---- Logs check
	emit(logsCheck(ctx, cfg, authResp))

	// ---- OTLP check
	emit(otlpCheck(ctx, cfg, authResp))

	// ---- Resources check
	emit(resourcesCheck(ctx, cfg, authResp))

	// ---- RPC check (config validity only — no live RPC poll in doctor)
	emit(rpcCheck(cfg, authResp))

	// ---- Health check (config validity only)
	emit(healthCheck(cfg))

	fmt.Fprintln(w)
	if hasRed {
		return 1
	}
	return 0
}

var metadataCheckNames = []string{"Metadata config", "Metadata conflicts"}

func metadataAllSame(status Status, detail string) []CheckResult {
	results := make([]CheckResult, len(metadataCheckNames))
	for i, name := range metadataCheckNames {
		results[i] = CheckResult{Name: name, Status: status, Detail: detail}
	}
	return results
}

func metadataChecks(cfg *config.Config, ar *AuthResponse) []CheckResult {
	if !cfg.Metadata.Enabled {
		return metadataAllSame(StatusOrange, "disabled in config")
	}
	if ar != nil && !slices.Contains(ar.Permissions, "metrics") {
		return metadataAllSame(StatusGrey, "metrics permission not granted")
	}
	return []CheckResult{
		CheckMetadataConfig(cfg.Metadata),
		CheckMetadataConflicts(cfg.Metadata),
	}
}

func logsCheck(ctx context.Context, cfg *config.Config, ar *AuthResponse) CheckResult {
	if !cfg.Logs.Enabled {
		return CheckResult{Name: "Logs", Status: StatusOrange, Detail: "disabled in config"}
	}
	if ar != nil && !slices.Contains(ar.Permissions, "logs") {
		return CheckResult{Name: "Logs", Status: StatusGrey, Detail: "logs permission not granted"}
	}
	src, err := logs.NewSource(cfg.Logs.Source, cfg.Logs.ContainerName, cfg.Logs.JournaldUnit, cfg.Logs.ResumeLookback.Duration)
	if err != nil {
		return CheckResult{Name: "Logs", Status: StatusRed, Detail: fmt.Sprintf("invalid source config: %v", err)}
	}
	minLevel := cfg.Logs.MinLevel
	if ar != nil && pkglogger.LevelRank(ar.LogsMinLevel) > pkglogger.LevelRank(minLevel) {
		minLevel = ar.LogsMinLevel
	}
	return CheckLogs(ctx, src, cfg.Logs, minLevel)
}

func otlpCheck(ctx context.Context, cfg *config.Config, ar *AuthResponse) CheckResult {
	if !cfg.OTLP.Enabled {
		return CheckResult{Name: "OTLP", Status: StatusOrange, Detail: "disabled in config"}
	}
	if ar != nil && !slices.Contains(ar.Permissions, "otlp") {
		return CheckResult{Name: "OTLP", Status: StatusGrey, Detail: "otlp permission not granted"}
	}
	return CheckOTLP(ctx, cfg.OTLP.ListenAddr)
}

func resourcesCheck(ctx context.Context, cfg *config.Config, ar *AuthResponse) CheckResult {
	if !cfg.Resources.Enabled {
		return CheckResult{Name: "Resources", Status: StatusOrange, Detail: "disabled in config"}
	}
	if ar != nil && !slices.Contains(ar.Permissions, "metrics") {
		return CheckResult{Name: "Resources", Status: StatusGrey, Detail: "metrics permission not granted"}
	}
	return CheckResources(ctx, cfg.Resources)
}

func rpcCheck(cfg *config.Config, ar *AuthResponse) CheckResult {
	if !cfg.RPC.Enabled {
		return CheckResult{Name: "RPC", Status: StatusOrange, Detail: "disabled in config"}
	}
	if ar != nil && !slices.Contains(ar.Permissions, "rpc") {
		return CheckResult{Name: "RPC", Status: StatusGrey, Detail: "rpc permission not granted"}
	}
	if cfg.RPC.RPCURL == "" {
		return CheckResult{Name: "RPC", Status: StatusRed, Detail: "rpc_url not set"}
	}
	return CheckResult{Name: "RPC", Status: StatusGreen, Detail: fmt.Sprintf("configured: %s", cfg.RPC.RPCURL)}
}

func healthCheck(cfg *config.Config) CheckResult {
	if !cfg.Health.Enabled {
		return CheckResult{Name: "Health", Status: StatusOrange, Detail: "disabled in config"}
	}
	if cfg.Health.ListenAddr == "" {
		return CheckResult{Name: "Health", Status: StatusRed, Detail: "listen_addr not set"}
	}
	return CheckResult{Name: "Health", Status: StatusGreen, Detail: fmt.Sprintf("configured: %s", cfg.Health.ListenAddr)}
}
