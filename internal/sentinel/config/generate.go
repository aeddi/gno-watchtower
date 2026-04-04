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
	afterKey string // insert after the line containing this key
	comment  string // comment line above the commented-out key
	key      string // the commented-out key name
	value    string // the commented-out value
}

// Generate detects the environment, builds a default config with detected values,
// and writes annotated TOML to stdout. Detection progress is printed to stderr.
func Generate(ctx context.Context, stderr, stdout io.Writer) error {
	cfg := DefaultConfig()
	env := Detect(ctx, stderr)
	applyDetection(cfg, env)

	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	output := injectAlternatives(string(data), buildAlternatives(cfg, env))
	_, err = io.WriteString(stdout, output)
	return err
}

// buildAlternatives returns the commented-out alternatives for the metadata section,
// based on which mode (docker vs native) was detected.
func buildAlternatives(cfg *Config, env *Environment) []alternative {
	var alts []alternative

	if env.Docker != nil {
		// Docker mode: cmd fields are active. Show path alternatives.
		alts = append(alts, alternative{
			afterKey: "binary_version_cmd",
			comment:  "Alternative: use binary path directly (runs <path> version)",
			key:      "binary_path",
			value:    "<path-to-gnoland>",
		})
		alts = append(alts, alternative{
			afterKey: "config_get_cmd",
			comment:  "Alternative: read config file directly",
			key:      "config_path",
			value:    "<path-to-gnoland-config>",
		})
	} else {
		// Native mode: path fields are active. Show cmd alternatives.
		alts = append(alts, alternative{
			afterKey: "binary_path",
			comment:  "Alternative: run a command to get the version (e.g. via docker exec)",
			key:      "binary_version_cmd",
			value:    "docker exec <container> gnoland version",
		})
		alts = append(alts, alternative{
			afterKey: "config_path",
			comment:  "Alternative: run a command to get config values (e.g. via docker exec)",
			key:      "config_get_cmd",
			value:    "docker exec <container> gnoland config get %s --raw",
		})
	}

	return alts
}

// injectAlternatives inserts commented-out key=value lines after their corresponding
// active keys in the TOML string.
func injectAlternatives(tomlStr string, alts []alternative) string {
	lines := strings.Split(tomlStr, "\n")
	var result []string

	for _, line := range lines {
		result = append(result, line)
		trimmed := strings.TrimSpace(line)
		for _, alt := range alts {
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
