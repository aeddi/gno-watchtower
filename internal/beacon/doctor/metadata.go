package doctor

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aeddi/gno-watchtower/internal/beacon/config"
	"github.com/aeddi/gno-watchtower/internal/sentinel/metadata"
)

// CheckMetadataConfig verifies the beacon can access the sentry's gnoland
// config, either by reading ConfigPath directly or by running ConfigGetCmd.
// The beacon/augment package uses sentinel/metadata's readers verbatim, so
// this check exercises the exact same code path the live beacon would — no
// shim, no mock.
func CheckMetadataConfig(ctx context.Context, cfg config.MetadataConfig) CheckResult {
	const name = "Metadata config"
	switch {
	case cfg.ConfigPath == "" && cfg.ConfigGetCmd == "":
		return CheckResult{Name: name, Status: StatusOrange, Detail: "not configured (augmentation disabled)"}
	case cfg.ConfigPath != "":
		if config.IsPlaceholder(cfg.ConfigPath) {
			return CheckResult{Name: name, Status: StatusOrange, Detail: "config_path not configured"}
		}
		f, err := os.Open(cfg.ConfigPath)
		if err != nil {
			return CheckResult{Name: name, Status: StatusRed, Detail: err.Error()}
		}
		f.Close()
		var found int
		for _, key := range metadata.ConfigKeys {
			if _, err := metadata.ReadConfigKey(cfg.ConfigPath, key); err == nil {
				found++
			}
		}
		return CheckResult{
			Name:   name,
			Status: StatusGreen,
			Detail: fmt.Sprintf("%d/%d keys found", found, len(metadata.ConfigKeys)),
		}
	default: // cmd
		firstKey := metadata.ConfigKeys[0]
		cmd := strings.ReplaceAll(cfg.ConfigGetCmd, "%s", firstKey)
		if _, err := metadata.RunCmd(ctx, cmd); err != nil {
			return CheckResult{Name: name, Status: StatusRed, Detail: err.Error()}
		}
		return CheckResult{Name: name, Status: StatusGreen, Detail: "cmd accessible"}
	}
}
