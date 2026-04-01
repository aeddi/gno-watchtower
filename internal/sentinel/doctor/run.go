// internal/sentinel/doctor/run.go
package doctor

import (
	"context"
	"fmt"
	"io"

	"github.com/gnolang/val-companion/internal/sentinel/config"
	"github.com/gnolang/val-companion/internal/sentinel/logs"
)

// Run executes all doctor checks, writes a formatted report to w, and returns an exit code.
// Exit code 0 = no Red results. Exit code 1 = at least one Red result.
func Run(ctx context.Context, cfg *config.Config, configPath string, w io.Writer) int {
	var results []CheckResult

	// ---- Remote + token + permissions (always run; drives Grey status below)
	remoteResults, authResp := CheckRemoteTokenAndPermissions(ctx, cfg)
	results = append(results, remoteResults...)

	// ---- Metadata checks
	results = append(results, metadataChecks(cfg, authResp)...)

	// ---- Logs check
	results = append(results, logsCheck(ctx, cfg, authResp))

	// ---- OTLP check
	results = append(results, otlpCheck(ctx, cfg, authResp))

	// ---- Resources check
	results = append(results, resourcesCheck(ctx, cfg, authResp))

	// ---- RPC check (config validity only — no live RPC poll in doctor)
	results = append(results, rpcCheck(cfg, authResp))

	PrintReport(w, configPath, results)

	for _, r := range results {
		if r.Status == StatusRed {
			return 1
		}
	}
	return 0
}

func metadataChecks(cfg *config.Config, ar *AuthResponse) []CheckResult {
	if !cfg.Metadata.Enabled {
		return []CheckResult{
			{Name: "Metadata binary", Status: StatusOrange, Detail: "disabled in config"},
			{Name: "Metadata genesis", Status: StatusOrange, Detail: "disabled in config"},
			{Name: "Metadata config", Status: StatusOrange, Detail: "disabled in config"},
			{Name: "Metadata conflicts", Status: StatusOrange, Detail: "disabled in config"},
		}
	}
	if ar != nil && !hasPermission(ar, "metrics") {
		return []CheckResult{
			{Name: "Metadata binary", Status: StatusGrey, Detail: "metrics permission not granted"},
			{Name: "Metadata genesis", Status: StatusGrey, Detail: "metrics permission not granted"},
			{Name: "Metadata config", Status: StatusGrey, Detail: "metrics permission not granted"},
			{Name: "Metadata conflicts", Status: StatusGrey, Detail: "metrics permission not granted"},
		}
	}
	return []CheckResult{
		CheckMetadataBinary(cfg.Metadata),
		CheckMetadataGenesis(cfg.Metadata),
		CheckMetadataConfig(cfg.Metadata),
		CheckMetadataConflicts(cfg.Metadata),
	}
}

func logsCheck(ctx context.Context, cfg *config.Config, ar *AuthResponse) CheckResult {
	if !cfg.Logs.Enabled {
		return CheckResult{Name: "Logs", Status: StatusOrange, Detail: "disabled in config"}
	}
	if ar != nil && !hasPermission(ar, "logs") {
		return CheckResult{Name: "Logs", Status: StatusGrey, Detail: "logs permission not granted"}
	}
	src, err := logs.NewSource(cfg.Logs.Source, cfg.Logs.ContainerName, cfg.Logs.JournaldUnit)
	if err != nil {
		return CheckResult{Name: "Logs", Status: StatusRed, Detail: fmt.Sprintf("invalid source config: %v", err)}
	}
	minLevel := cfg.Logs.MinLevel
	if ar != nil && levelRank(ar.LogsMinLevel) > levelRank(minLevel) {
		minLevel = ar.LogsMinLevel
	}
	return CheckLogs(ctx, src, cfg.Logs, minLevel)
}

func otlpCheck(ctx context.Context, cfg *config.Config, ar *AuthResponse) CheckResult {
	if !cfg.OTLP.Enabled {
		return CheckResult{Name: "OTLP", Status: StatusOrange, Detail: "disabled in config"}
	}
	if ar != nil && !hasPermission(ar, "otlp") {
		return CheckResult{Name: "OTLP", Status: StatusGrey, Detail: "otlp permission not granted"}
	}
	return CheckOTLP(ctx, cfg.OTLP.ListenAddr)
}

func resourcesCheck(ctx context.Context, cfg *config.Config, ar *AuthResponse) CheckResult {
	if !cfg.Resources.Enabled {
		return CheckResult{Name: "Resources", Status: StatusOrange, Detail: "disabled in config"}
	}
	if ar != nil && !hasPermission(ar, "metrics") {
		return CheckResult{Name: "Resources", Status: StatusGrey, Detail: "metrics permission not granted"}
	}
	return CheckResources(ctx, cfg.Resources)
}

func rpcCheck(cfg *config.Config, ar *AuthResponse) CheckResult {
	if !cfg.RPC.Enabled {
		return CheckResult{Name: "RPC", Status: StatusOrange, Detail: "disabled in config"}
	}
	if ar != nil && !hasPermission(ar, "rpc") {
		return CheckResult{Name: "RPC", Status: StatusGrey, Detail: "rpc permission not granted"}
	}
	if cfg.RPC.RPCURL == "" {
		return CheckResult{Name: "RPC", Status: StatusRed, Detail: "rpc_url not set"}
	}
	return CheckResult{Name: "RPC", Status: StatusGreen, Detail: fmt.Sprintf("configured: %s", cfg.RPC.RPCURL)}
}
