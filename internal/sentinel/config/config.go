package config

import (
	"fmt"
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

type Config struct {
	Server ServerConfig `toml:"server"`
	RPC    RPCConfig    `toml:"rpc"`
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

func Load(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("load config %s: %w", path, err)
	}
	return &cfg, nil
}
