package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Duration wraps time.Duration to support TOML string values like "3s", "30s".
type Duration struct{ time.Duration }

func (d *Duration) UnmarshalText(text []byte) error {
	dur, err := time.ParseDuration(string(text))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", text, err)
	}
	d.Duration = dur
	return nil
}

func (d Duration) MarshalText() ([]byte, error) {
	return []byte(d.Duration.String()), nil
}

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

type Config struct {
	Server ServerConfig `toml:"server"`
	RPC    RPCConfig    `toml:"rpc"`
	Logs   LogsConfig   `toml:"logs"`
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

func Load(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("load config %s: %w", path, err)
	}
	return &cfg, nil
}
