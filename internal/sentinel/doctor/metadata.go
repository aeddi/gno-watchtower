// internal/sentinel/doctor/metadata.go
package doctor

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aeddi/gno-watchtower/internal/sentinel/config"
	"github.com/aeddi/gno-watchtower/internal/sentinel/metadata"
)

// CheckMetadataConfig checks whether the config file is accessible (path mode)
// or the config command runs without error (cmd mode). The path/cmd-both-set
// conflict is rejected earlier by config.Load and never reaches this check.
func CheckMetadataConfig(ctx context.Context, cfg config.MetadataConfig) CheckResult {
	const name = "Metadata config"
	switch {
	case cfg.ConfigPath == "" && cfg.ConfigGetCmd == "":
		return CheckResult{Name: name, Status: StatusOrange, Detail: "not configured"}
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
