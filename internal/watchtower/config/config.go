package config

import (
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/gnolang/val-companion/pkg/tomlutil"
)

// Duration wraps time.Duration for TOML string unmarshaling.
type Duration = tomlutil.Duration

// ValidatorConfig holds per-validator settings.
type ValidatorConfig struct {
	Token        string   `toml:"token"`
	Permissions  []string `toml:"permissions"`
	LogsMinLevel string   `toml:"logs_min_level"`
}

// TokenEntry is a pre-built lookup entry mapping a token to its validator.
type TokenEntry struct {
	ValidatorName string
	Config        ValidatorConfig
}

// Config is the root watchtower configuration.
type Config struct {
	Server          ServerConfig          `toml:"server"`
	Security        SecurityConfig
	VictoriaMetrics VictoriaMetricsConfig `toml:"victoria_metrics"`
	Loki            LokiConfig
	Validators      map[string]ValidatorConfig `toml:"validators"`

	// TokenIndex is built from Validators at load time; not in TOML.
	TokenIndex map[string]TokenEntry `toml:"-"`
}

type ServerConfig struct {
	ListenAddr string `toml:"listen_addr"`
}

type SecurityConfig struct {
	RateLimitRPS float64  `toml:"rate_limit_rps"`
	BanThreshold int      `toml:"ban_threshold"`
	BanDuration  Duration `toml:"ban_duration"`
}

type VictoriaMetricsConfig struct {
	URL string `toml:"url"`
}

type LokiConfig struct {
	URL string `toml:"url"`
}

// Load parses the TOML config file at path and returns the populated Config.
func Load(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("load config %s: %w", path, err)
	}
	cfg.TokenIndex = make(map[string]TokenEntry, len(cfg.Validators))
	for name, v := range cfg.Validators {
		cfg.TokenIndex[v.Token] = TokenEntry{ValidatorName: name, Config: v}
	}
	return &cfg, nil
}
