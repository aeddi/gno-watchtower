package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/gnolang/val-companion/pkg/tomlutil"
)

// Duration wraps time.Duration to support TOML string values like "3s", "30s".
type Duration = tomlutil.Duration

// ByteSize is an int64 that unmarshals from TOML strings like "1MB", "512KB", "1GB".
// Plain integers are also accepted (e.g. "1024" = 1024 bytes).
type ByteSize int64

func (b *ByteSize) UnmarshalText(text []byte) error {
	s := strings.ToUpper(strings.TrimSpace(string(text)))
	for _, m := range []struct {
		suffix string
		mult   int64
	}{
		{"GB", 1024 * 1024 * 1024},
		{"MB", 1024 * 1024},
		{"KB", 1024},
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
	case v > 0 && v%(1024*1024*1024) == 0:
		return []byte(fmt.Sprintf("%dGB", v/(1024*1024*1024))), nil
	case v > 0 && v%(1024*1024) == 0:
		return []byte(fmt.Sprintf("%dMB", v/(1024*1024))), nil
	case v > 0 && v%1024 == 0:
		return []byte(fmt.Sprintf("%dKB", v/1024)), nil
	default:
		return []byte(strconv.FormatInt(v, 10)), nil
	}
}

type Config struct {
	Server    ServerConfig    `toml:"server"`
	RPC       RPCConfig       `toml:"rpc"`
	Logs      LogsConfig      `toml:"logs"`
	OTLP      OTLPConfig      `toml:"otlp"`
	Resources ResourcesConfig `toml:"resources"`
	Metadata  MetadataConfig  `toml:"metadata"`
	Health    HealthConfig    `toml:"health"`
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
	Source        string   `toml:"source"`         // "docker" | "journald"
	ContainerName string   `toml:"container_name"` // docker only
	JournaldUnit  string   `toml:"journald_unit"`  // journald only
	BatchSize     ByteSize `toml:"batch_size"`
	BatchTimeout  Duration `toml:"batch_timeout"`
	MinLevel      string   `toml:"min_level"` // "debug" | "info" | "warn" | "error"
}

// OTLPConfig holds OTLP relay settings.
type OTLPConfig struct {
	Enabled    bool   `toml:"enabled"`
	ListenAddr string `toml:"listen_addr"` // default: "localhost:4317"
}

// ResourcesConfig holds resource monitor settings.
type ResourcesConfig struct {
	Enabled       bool     `toml:"enabled"`
	PollInterval  Duration `toml:"poll_interval"`
	Source        string   `toml:"source"`         // "host" | "docker" | "both"
	ContainerName string   `toml:"container_name"` // docker only
}

// MetadataConfig holds metadata collector settings.
// For each item, set exactly one of _path or _cmd — setting both is an error detected at runtime.
type MetadataConfig struct {
	Enabled       bool     `toml:"enabled"`
	CheckInterval Duration `toml:"check_interval"`

	BinaryPath        string `toml:"binary_path"`
	BinaryChecksumCmd string `toml:"binary_checksum_cmd"`

	ConfigPath   string `toml:"config_path"`
	ConfigGetCmd string `toml:"config_get_cmd"` // use %s as placeholder for key name

	GenesisPath        string `toml:"genesis_path"`
	GenesisChecksumCmd string `toml:"genesis_checksum_cmd"`
}

// HealthConfig holds the sentinel health endpoint settings.
// If ListenAddr is empty, no health endpoint is started.
type HealthConfig struct {
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
	return nil
}
