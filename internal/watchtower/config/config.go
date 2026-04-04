package config

import (
	"fmt"
	"os"

	toml "github.com/pelletier/go-toml/v2"

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
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load config %s: %w", path, err)
	}
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("load config %s: %w", path, err)
	}
	if cfg.Security.RateLimitBurst == 0 {
		cfg.Security.RateLimitBurst = 10
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if c.Server.ListenAddr == "" {
		return fmt.Errorf("server.listen_addr is required")
	}
	if c.VictoriaMetrics.URL == "" {
		return fmt.Errorf("victoria_metrics.url is required")
	}
	if c.Loki.URL == "" {
		return fmt.Errorf("loki.url is required")
	}
	if c.Security.RateLimitRPS <= 0 {
		return fmt.Errorf("security.rate_limit_rps must be > 0")
	}
	if c.Security.BanThreshold <= 0 {
		return fmt.Errorf("security.ban_threshold must be > 0")
	}
	if c.Security.BanDuration.Duration <= 0 {
		return fmt.Errorf("security.ban_duration must be > 0")
	}
	for name, v := range c.Validators {
		if v.Token == "" {
			return fmt.Errorf("validators.%s.token is required", name)
		}
	}
	return nil
}
