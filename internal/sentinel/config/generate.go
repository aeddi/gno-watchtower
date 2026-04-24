package config

import (
	"context"
	"fmt"
	"io"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

// alternative describes a commented-out config key to inject after an active key.
type alternative struct {
	section  string // only match within this TOML section (empty = any)
	afterKey string // insert after the line containing this key
	comment  string // comment line above the commented-out key
	key      string // the commented-out key name
	value    string // the commented-out value
}

// Generate detects the environment, builds a default config with detected values,
// and writes annotated TOML to stdout. Detection progress is printed to stderr.
func Generate(ctx context.Context, progress, output io.Writer) error {
	cfg := DefaultConfig()
	env := Detect(ctx, progress)
	fmt.Fprintln(progress)
	applyDetection(cfg, env)

	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	result := injectAlternatives(string(data), buildAlternatives(cfg, env))
	_, err = io.WriteString(output, result)
	return err
}

// buildAlternatives returns the commented-out alternatives for the metadata section,
// based on which mode (docker vs native) was detected.
func buildAlternatives(cfg *Config, env *Environment) []alternative {
	var alts []alternative

	// ---- Logs source alternatives
	if cfg.Logs.Source == LogSourceDocker {
		alts = append(alts, alternative{
			section:  "logs",
			afterKey: "container_name",
			comment:  "Alternative: use journald as log source",
			key:      "journald_unit",
			value:    placeholderJournaldUnit,
		})
	} else if cfg.Logs.Source == LogSourceJournald {
		alts = append(alts, alternative{
			section:  "logs",
			afterKey: "journald_unit",
			comment:  "Alternative: use docker as log source",
			key:      "container_name",
			value:    placeholderContainerName,
		})
	}

	// ---- Metadata config_get_cmd alternative to config_path
	// config_path is always the primary — config_get_cmd requires a docker (or
	// other) CLI reachable from the sentinel process, which doesn't hold in
	// containerised sentinel deployments. In docker mode we pre-fill the
	// alternative with the detected container name to save one edit for
	// operators who are running the sentinel on the host.
	cmdValue := "docker exec <container> gnoland config get %s --raw"
	if env.Docker != nil {
		cmdValue = fmt.Sprintf("docker exec %s gnoland config get %%s --raw", env.Docker.ContainerName)
	}
	alts = append(alts, alternative{
		afterKey: "config_path",
		comment:  "Alternative: run a shell command (requires the command to be reachable from the sentinel process — not the case for containerised sentinels without a bundled CLI)",
		key:      "config_get_cmd",
		value:    cmdValue,
	})

	return alts
}

// injectAlternatives inserts commented-out key=value lines after their corresponding
// active keys in the TOML string. If an alternative has a section constraint, it only
// matches within that section.
func injectAlternatives(tomlStr string, alts []alternative) string {
	lines := strings.Split(tomlStr, "\n")
	var result []string
	var currentSection string

	for _, line := range lines {
		result = append(result, line)
		trimmed := strings.TrimSpace(line)

		// Track current TOML section.
		if strings.HasPrefix(trimmed, "[") && !strings.HasPrefix(trimmed, "[[") {
			currentSection = strings.Trim(trimmed, "[] ")
		}

		for _, alt := range alts {
			if alt.section != "" && alt.section != currentSection {
				continue
			}
			prefix := alt.afterKey + " ="
			prefixNoSpace := alt.afterKey + "="
			if strings.HasPrefix(trimmed, prefix) || strings.HasPrefix(trimmed, prefixNoSpace) {
				result = append(result, fmt.Sprintf("# %s", alt.comment))
				result = append(result, fmt.Sprintf("# %s = '%s'", alt.key, alt.value))
			}
		}
	}

	return strings.Join(result, "\n")
}
