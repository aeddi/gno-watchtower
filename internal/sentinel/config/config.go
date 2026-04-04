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
	Server    ServerConfig    `toml:"server" comment:"Connection to the watchtower server"`
	RPC       RPCConfig       `toml:"rpc" comment:"RPC status collector"`
	Logs      LogsConfig      `toml:"logs" comment:"Log collector"`
	OTLP      OTLPConfig      `toml:"otlp" comment:"OpenTelemetry relay"`
	Resources ResourcesConfig `toml:"resources" comment:"System resource monitor"`
	Metadata  MetadataConfig  `toml:"metadata" comment:"Binary, genesis, and config metadata collector"`
	Health    HealthConfig    `toml:"health" comment:"Sentinel health endpoint"`
}

type ServerConfig struct {
	URL   string `toml:"url" comment:"Watchtower server URL"`
	Token string `toml:"token" comment:"Authentication token for this validator"`
}

type RPCConfig struct {
	Enabled                    bool     `toml:"enabled" comment:"Enable the RPC collector"`
	PollInterval               Duration `toml:"poll_interval" comment:"Polling interval for RPC status"`
	RPCURL                     string   `toml:"rpc_url" comment:"Gnoland RPC endpoint URL"`
	DumpConsensusStateInterval Duration `toml:"dump_consensus_state_interval" comment:"Interval for dump_consensus_state polling"`
}

// LogsConfig holds log collector settings.
type LogsConfig struct {
	Enabled       bool     `toml:"enabled" comment:"Enable the log collector"`
	Source        string   `toml:"source" comment:"Log source: docker or journald"`
	ContainerName string   `toml:"container_name,omitempty" comment:"Docker container name (docker source only)"`
	JournaldUnit  string   `toml:"journald_unit,omitempty" comment:"Systemd unit name (journald source only)"`
	BatchSize     ByteSize `toml:"batch_size" comment:"Maximum batch size before sending"`
	BatchTimeout  Duration `toml:"batch_timeout" comment:"Maximum time to wait before sending a batch"`
	MinLevel      string   `toml:"min_level" comment:"Minimum log level: debug, info, warn, or error"`
}

// OTLPConfig holds OTLP relay settings.
type OTLPConfig struct {
	Enabled    bool   `toml:"enabled" comment:"Enable the OTLP relay"`
	ListenAddr string `toml:"listen_addr" comment:"gRPC listen address for OTLP receiver"`
}

// ResourcesConfig holds resource monitor settings.
type ResourcesConfig struct {
	Enabled       bool     `toml:"enabled" comment:"Enable the resource monitor"`
	PollInterval  Duration `toml:"poll_interval" comment:"Polling interval for resource metrics"`
	Source        string   `toml:"source" comment:"Resource source: host, docker, or both"`
	ContainerName string   `toml:"container_name,omitempty" comment:"Docker container name (docker/both source only)"`
}

// MetadataConfig holds metadata collector settings.
// For binary and config: set exactly one of _path or _cmd — setting both is an error.
type MetadataConfig struct {
	Enabled       bool     `toml:"enabled" comment:"Enable the metadata collector"`
	CheckInterval Duration `toml:"check_interval" comment:"Check interval for polling-based items"`

	BinaryPath       string `toml:"binary_path,omitempty" comment:"Path to the gnoland binary (runs <path> version)"`
	BinaryVersionCmd string `toml:"binary_version_cmd,omitempty" comment:"Command to get gnoland binary version"`

	ConfigPath   string `toml:"config_path,omitempty" comment:"Path to the gnoland config file"`
	ConfigGetCmd string `toml:"config_get_cmd,omitempty" comment:"Command to get gnoland config values (%s = key name)"`

	GenesisPath string `toml:"genesis_path" comment:"Path to the genesis.json file"`
}

// HealthConfig holds the sentinel health endpoint settings.
// If ListenAddr is empty, no health endpoint is started.
type HealthConfig struct {
	ListenAddr string `toml:"listen_addr,omitempty" comment:"HTTP listen address (empty = disabled)"`
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
