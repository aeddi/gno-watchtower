// internal/sentinel/doctor/resources.go
package doctor

import (
	"context"
	"fmt"
	"strings"

	gopsutilcpu "github.com/shirou/gopsutil/v3/cpu"
	gopsutildisk "github.com/shirou/gopsutil/v3/disk"
	gopsutilmem "github.com/shirou/gopsutil/v3/mem"
	gopsutilnet "github.com/shirou/gopsutil/v3/net"

	"github.com/gnolang/val-companion/internal/sentinel/config"
	"github.com/gnolang/val-companion/internal/sentinel/resources"
)

// CheckResources verifies that host and/or container resource stats can be read.
func CheckResources(ctx context.Context, cfg config.ResourcesConfig) CheckResult {
	const name = "Resources"

	if cfg.Source == "" {
		return CheckResult{Name: name, Status: StatusOrange, Detail: "source not configured"}
	}

	var errs []string

	if cfg.Source == "host" || cfg.Source == "both" {
		if err := checkHostStats(ctx); err != nil {
			errs = append(errs, fmt.Sprintf("host: %v", err))
		}
	}

	if cfg.Source == "docker" || cfg.Source == "both" {
		if _, err := resources.ContainerStats(ctx, cfg.ContainerName); err != nil {
			errs = append(errs, fmt.Sprintf("docker: %v", err))
		}
	}

	if len(errs) > 0 {
		return CheckResult{Name: name, Status: StatusRed, Detail: strings.Join(errs, "; ")}
	}
	return CheckResult{Name: name, Status: StatusGreen, Detail: fmt.Sprintf("source=%s OK", cfg.Source)}
}

// checkHostStats calls each gopsutil function once to verify access.
func checkHostStats(ctx context.Context) error {
	if _, err := gopsutilcpu.PercentWithContext(ctx, 0, false); err != nil {
		return fmt.Errorf("cpu: %w", err)
	}
	if _, err := gopsutilmem.VirtualMemoryWithContext(ctx); err != nil {
		return fmt.Errorf("memory: %w", err)
	}
	if _, err := gopsutildisk.UsageWithContext(ctx, "/"); err != nil {
		return fmt.Errorf("disk: %w", err)
	}
	if _, err := gopsutilnet.IOCountersWithContext(ctx, false); err != nil {
		return fmt.Errorf("network: %w", err)
	}
	return nil
}
