// internal/sentinel/doctor/metadata.go
package doctor

import (
	"fmt"
	"os"
	"strings"

	"github.com/gnolang/val-companion/internal/sentinel/config"
	"github.com/gnolang/val-companion/internal/sentinel/metadata"
)

// CheckMetadataBinary checks whether the binary checksum can be computed.
func CheckMetadataBinary(cfg config.MetadataConfig) CheckResult {
	const name = "Metadata binary"
	switch {
	case cfg.BinaryPath == "" && cfg.BinaryChecksumCmd == "":
		return CheckResult{Name: name, Status: StatusOrange, Detail: "not configured"}
	case cfg.BinaryPath != "" && cfg.BinaryChecksumCmd != "":
		return CheckResult{Name: name, Status: StatusOrange, Detail: "skipped: see Metadata conflicts"}
	case cfg.BinaryPath != "":
		sum, err := metadata.SHA256File(cfg.BinaryPath)
		if err != nil {
			return CheckResult{Name: name, Status: StatusRed, Detail: err.Error()}
		}
		return CheckResult{Name: name, Status: StatusGreen, Detail: fmt.Sprintf("sha256: %s", sum[:16]+"...")}
	default: // cmd
		out, err := metadata.RunCmd(cfg.BinaryChecksumCmd)
		if err != nil {
			return CheckResult{Name: name, Status: StatusRed, Detail: err.Error()}
		}
		return CheckResult{Name: name, Status: StatusGreen, Detail: fmt.Sprintf("cmd output: %s", truncate(out, 40))}
	}
}

// CheckMetadataGenesis checks whether the genesis checksum can be computed.
func CheckMetadataGenesis(cfg config.MetadataConfig) CheckResult {
	const name = "Metadata genesis"
	switch {
	case cfg.GenesisPath == "" && cfg.GenesisChecksumCmd == "":
		return CheckResult{Name: name, Status: StatusOrange, Detail: "not configured"}
	case cfg.GenesisPath != "" && cfg.GenesisChecksumCmd != "":
		return CheckResult{Name: name, Status: StatusOrange, Detail: "skipped: see Metadata conflicts"}
	case cfg.GenesisPath != "":
		sum, err := metadata.SHA256File(cfg.GenesisPath)
		if err != nil {
			return CheckResult{Name: name, Status: StatusRed, Detail: err.Error()}
		}
		return CheckResult{Name: name, Status: StatusGreen, Detail: fmt.Sprintf("sha256: %s", sum[:16]+"...")}
	default: // cmd
		out, err := metadata.RunCmd(cfg.GenesisChecksumCmd)
		if err != nil {
			return CheckResult{Name: name, Status: StatusRed, Detail: err.Error()}
		}
		return CheckResult{Name: name, Status: StatusGreen, Detail: fmt.Sprintf("cmd output: %s", truncate(out, 40))}
	}
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
	if cfg.BinaryPath != "" && cfg.BinaryChecksumCmd != "" {
		conflicts = append(conflicts, "binary (binary_path + binary_checksum_cmd)")
	}
	if cfg.GenesisPath != "" && cfg.GenesisChecksumCmd != "" {
		conflicts = append(conflicts, "genesis (genesis_path + genesis_checksum_cmd)")
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
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
