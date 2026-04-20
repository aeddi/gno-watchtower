package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/aeddi/gno-watchtower/pkg/tomlutil"
)

// Duration wraps time.Duration to support TOML string values like "3s", "30s".
type Duration = tomlutil.Duration

// Byte size multipliers.
const (
	KB = 1024
	MB = 1024 * KB
	GB = 1024 * MB
)

// Log source values.
const (
	LogSourceDocker   = "docker"
	LogSourceJournald = "journald"
)

// Resource source values.
const (
	ResSourceHost   = "host"
	ResSourceDocker = "docker"
	ResSourceBoth   = "both"
)

// ByteSize is an int64 that unmarshals from TOML strings like "1MB", "512KB", "1GB".
// Plain integers are also accepted (e.g. "1024" = 1024 bytes).
type ByteSize int64

func (b *ByteSize) UnmarshalText(text []byte) error {
	s := strings.ToUpper(strings.TrimSpace(string(text)))
	for _, m := range []struct {
		suffix string
		mult   int64
	}{
		{"GB", GB},
		{"MB", MB},
		{"KB", KB},
	} {
		if strings.HasSuffix(s, m.suffix) {
			n, err := strconv.ParseInt(strings.TrimSuffix(s, m.suffix), 10, 64)
			if err != nil {
				return fmt.Errorf("invalid byte size %q: %w", string(text), err)
			}
			*b = ByteSize(n * m.mult)
			return nil
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid byte size %q: %w", string(text), err)
	}
	*b = ByteSize(n)
	return nil
}

func (b ByteSize) MarshalText() ([]byte, error) {
	v := int64(b)
	switch {
	case v > 0 && v%GB == 0:
		return fmt.Appendf(nil, "%dGB", v/GB), nil
	case v > 0 && v%MB == 0:
		return fmt.Appendf(nil, "%dMB", v/MB), nil
	case v > 0 && v%KB == 0:
		return fmt.Appendf(nil, "%dKB", v/KB), nil
	default:
		return strconv.AppendInt(nil, v, 10), nil
	}
}

// Placeholder values used in generated configs for fields that need user input.
const (
	PlaceholderServerURL     = "<watchtower-server-url>"
	PlaceholderServerToken   = "<watchtower-auth-token>"
	PlaceholderContainerName = "<gnoland-container-name>"
	PlaceholderBinaryPath    = "<path-to-gnoland>"
	PlaceholderConfigPath    = "<path-to-gnoland-config>"
	PlaceholderGenesisPath   = "<path-to-genesis-json>"
	PlaceholderJournaldUnit  = "<gnoland-systemd-unit>"
)

// IsPlaceholder reports whether s is an unresolved placeholder value.
func IsPlaceholder(s string) bool {
	return len(s) > 2 && s[0] == '<' && s[len(s)-1] == '>'
}

type Config struct {
	Server    ServerConfig    `toml:"server" comment:"Connection to the watchtower server"`
	RPC       RPCConfig       `toml:"rpc" comment:"RPC status collector"`
	Logs      LogsConfig      `toml:"logs" comment:"Log collector"`
	OTLP      OTLPConfig      `toml:"otlp" comment:"OpenTelemetry relay"`
	Resources ResourcesConfig `toml:"resources" comment:"System resource monitor"`
	Metadata  MetadataConfig  `toml:"metadata" comment:"Binary, genesis, and config metadata collector"`
	Health    HealthConfig    `toml:"health" comment:"Sentinel health endpoint"`
}

type ServerConfig struct {
	URL   string `toml:"url"`
	Token string `toml:"token"`
}

type RPCConfig struct {
	Enabled                    bool     `toml:"enabled"`
	PollInterval               Duration `toml:"poll_interval"`
	RPCURL                     string   `toml:"rpc_url"`
	DumpConsensusStateInterval Duration `toml:"dump_consensus_state_interval"`
}

// LogsConfig holds log collector settings.
type LogsConfig struct {
	Enabled       bool     `toml:"enabled"`
	Source        string   `toml:"source" comment:"docker or journald"`
	ContainerName string   `toml:"container_name,omitempty" comment:"docker source only"`
	JournaldUnit  string   `toml:"journald_unit,omitempty" comment:"journald source only"`
	BatchSize     ByteSize `toml:"batch_size"`
	BatchTimeout  Duration `toml:"batch_timeout"`
	MinLevel      string   `toml:"min_level" comment:"debug, info, warn, or error"`
}

// OTLPConfig holds OTLP relay settings.
type OTLPConfig struct {
	Enabled    bool   `toml:"enabled"`
	ListenAddr string `toml:"listen_addr"`
}

// ResourcesConfig holds resource monitor settings.
type ResourcesConfig struct {
	Enabled       bool     `toml:"enabled"`
	PollInterval  Duration `toml:"poll_interval"`
	Source        string   `toml:"source" comment:"host, docker, or both"`
	ContainerName string   `toml:"container_name,omitempty"`
}

// MetadataConfig holds metadata collector settings.
// For binary and config: set exactly one of _path or _cmd — setting both is an error.
type MetadataConfig struct {
	Enabled       bool     `toml:"enabled"`
	CheckInterval Duration `toml:"check_interval"`

	BinaryPath       string `toml:"binary_path,omitempty" comment:"runs <path> version to get the binary version"`
	BinaryVersionCmd string `toml:"binary_version_cmd,omitempty"`

	ConfigPath   string `toml:"config_path,omitempty"`
	ConfigGetCmd string `toml:"config_get_cmd,omitempty" comment:"use %s as placeholder for the config key name"`

	GenesisPath string `toml:"genesis_path"`
}

// HealthConfig holds the sentinel health endpoint settings.
type HealthConfig struct {
	Enabled    bool   `toml:"enabled"`
	ListenAddr string `toml:"listen_addr"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load config %s: %w", path, err)
	}
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("load config %s: %w", path, err)
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return &cfg, nil
}

// DefaultConfig returns a Config with sensible defaults.
// Unknown values use angle-bracket placeholders.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			URL:   PlaceholderServerURL,
			Token: PlaceholderServerToken,
		},
		RPC: RPCConfig{
			Enabled:                    true,
			PollInterval:               Duration{Duration: 3 * time.Second},
			RPCURL:                     "http://localhost:26657",
			DumpConsensusStateInterval: Duration{Duration: 30 * time.Second},
		},
		Logs: LogsConfig{
			Enabled:       true,
			Source:        LogSourceDocker,
			ContainerName: PlaceholderContainerName,
			BatchSize:     ByteSize(MB),
			BatchTimeout:  Duration{Duration: 5 * time.Second},
			MinLevel:      "info",
		},
		OTLP: OTLPConfig{
			Enabled:    true,
			ListenAddr: "localhost:4317",
		},
		Resources: ResourcesConfig{
			Enabled:       true,
			PollInterval:  Duration{Duration: 10 * time.Second},
			Source:        ResSourceHost,
			ContainerName: PlaceholderContainerName,
		},
		Metadata: MetadataConfig{
			Enabled:       true,
			CheckInterval: Duration{Duration: 10 * time.Minute},
			BinaryPath:    PlaceholderBinaryPath,
			ConfigPath:    PlaceholderConfigPath,
			GenesisPath:   PlaceholderGenesisPath,
		},
		Health: HealthConfig{
			ListenAddr: "127.0.0.1:8081",
		},
	}
}

func (c *Config) anyCollectorEnabled() bool {
	return c.RPC.Enabled || c.Logs.Enabled || c.OTLP.Enabled || c.Resources.Enabled || c.Metadata.Enabled
}

func (c *Config) validate() error {
	if c.anyCollectorEnabled() {
		if c.Server.URL == "" {
			return fmt.Errorf("server.url is required when a collector is enabled")
		}
		if c.Server.Token == "" {
			return fmt.Errorf("server.token is required when a collector is enabled")
		}
	}
	if c.Health.Enabled && c.Health.ListenAddr == "" {
		return fmt.Errorf("health.listen_addr is required when health is enabled")
	}
	return nil
}
