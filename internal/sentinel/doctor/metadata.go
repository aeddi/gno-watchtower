// internal/sentinel/doctor/metadata.go
package doctor

import (
	"fmt"
	"os"
	"strings"

	"github.com/aeddi/gno-watchtower/internal/sentinel/config"
	"github.com/aeddi/gno-watchtower/internal/sentinel/metadata"
)

// CheckMetadataBinary checks whether the binary version can be retrieved.
func CheckMetadataBinary(cfg config.MetadataConfig) CheckResult {
	const name = "Metadata binary"
	switch {
	case cfg.BinaryPath == "" && cfg.BinaryVersionCmd == "":
		return CheckResult{Name: name, Status: StatusOrange, Detail: "not configured"}
	case cfg.BinaryPath != "" && cfg.BinaryVersionCmd != "":
		return CheckResult{Name: name, Status: StatusOrange, Detail: "skipped: see Metadata conflicts"}
	case cfg.BinaryPath != "":
		if config.IsPlaceholder(cfg.BinaryPath) {
			return CheckResult{Name: name, Status: StatusOrange, Detail: "binary_path not configured"}
		}
		if _, err := os.Stat(cfg.BinaryPath); err != nil {
			return CheckResult{Name: name, Status: StatusRed, Detail: err.Error()}
		}
		version, err := metadata.RunBinaryVersion(cfg.BinaryPath)
		if err != nil {
			// TODO: Remove this fallback once gnoland implements the version subcommand.
			return CheckResult{Name: name, Status: StatusGreen, Detail: "binary found, version not implemented yet"}
		}
		return CheckResult{Name: name, Status: StatusGreen, Detail: fmt.Sprintf("version: %s", truncate(version, 40))}
	default: // cmd
		out, err := metadata.RunCmd(cfg.BinaryVersionCmd)
		if err != nil {
			// TODO: Remove this fallback once gnoland implements the version subcommand.
			return CheckResult{Name: name, Status: StatusGreen, Detail: "cmd reachable, version not implemented yet"}
		}
		return CheckResult{Name: name, Status: StatusGreen, Detail: fmt.Sprintf("version: %s", truncate(out, 40))}
	}
}

// CheckMetadataGenesis checks whether the genesis checksum can be computed.
func CheckMetadataGenesis(cfg config.MetadataConfig) CheckResult {
	const name = "Metadata genesis"
	if cfg.GenesisPath == "" {
		return CheckResult{Name: name, Status: StatusOrange, Detail: "not configured"}
	}
	if config.IsPlaceholder(cfg.GenesisPath) {
		return CheckResult{Name: name, Status: StatusOrange, Detail: "genesis_path not configured"}
	}
	sum, err := metadata.SHA256File(cfg.GenesisPath)
	if err != nil {
		return CheckResult{Name: name, Status: StatusRed, Detail: err.Error()}
	}
	return CheckResult{Name: name, Status: StatusGreen, Detail: fmt.Sprintf("sha256: %s", sum[:16]+"...")}
}

// CheckMetadataConfig checks whether the config file is accessible (path mode)
// or the config command runs without error (cmd mode).
func CheckMetadataConfig(cfg config.MetadataConfig) CheckResult {
	const name = "Metadata config"
	switch {
	case cfg.ConfigPath == "" && cfg.ConfigGetCmd == "":
		return CheckResult{Name: name, Status: StatusOrange, Detail: "not configured"}
	case cfg.ConfigPath != "" && cfg.ConfigGetCmd != "":
		return CheckResult{Name: name, Status: StatusOrange, Detail: "skipped: see Metadata conflicts"}
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
		if _, err := metadata.RunCmd(cmd); err != nil {
			return CheckResult{Name: name, Status: StatusRed, Detail: err.Error()}
		}
		return CheckResult{Name: name, Status: StatusGreen, Detail: "cmd accessible"}
	}
}

// CheckMetadataConflicts detects path+cmd conflicts in the metadata config.
func CheckMetadataConflicts(cfg config.MetadataConfig) CheckResult {
	const name = "Metadata conflicts"
	var conflicts []string
	if cfg.BinaryPath != "" && cfg.BinaryVersionCmd != "" {
		conflicts = append(conflicts, "binary (binary_path + binary_version_cmd)")
	}
	if cfg.ConfigPath != "" && cfg.ConfigGetCmd != "" {
		conflicts = append(conflicts, "config (config_path + config_get_cmd)")
	}
	if len(conflicts) > 0 {
		return CheckResult{
			Name:   name,
			Status: StatusRed,
			Detail: fmt.Sprintf("conflicts detected: %s", strings.Join(conflicts, "; ")),
		}
	}
	return CheckResult{Name: name, Status: StatusGreen, Detail: "no conflicts"}
}

// truncate shortens s to at most n chars, appending "..." if truncated.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}
