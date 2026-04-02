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
	Server          ServerConfig               `toml:"server"`
	Security        SecurityConfig             `toml:"security"`
	VictoriaMetrics VictoriaMetricsConfig      `toml:"victoria_metrics"`
	Loki            LokiConfig                 `toml:"loki"`
	Validators      map[string]ValidatorConfig `toml:"validators"`
}

type ServerConfig struct {
	ListenAddr string `toml:"listen_addr"`
}

type SecurityConfig struct {
	RateLimitRPS   float64  `toml:"rate_limit_rps"`
	// RateLimitBurst is the maximum number of requests allowed in a burst.
	// Must be at least the number of data types sentinel sends concurrently
	// (rpc + metrics + logs + otlp = 4). Default 10.
	RateLimitBurst int      `toml:"rate_limit_burst"`
	BanThreshold   int      `toml:"ban_threshold"`
	BanDuration    Duration `toml:"ban_duration"`
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
	if cfg.Security.RateLimitBurst == 0 {
		cfg.Security.RateLimitBurst = 10
	}
	return &cfg, nil
}
