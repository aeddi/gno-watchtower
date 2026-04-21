package config

import (
	"fmt"
	"io"
	"os"
	"time"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/aeddi/gno-watchtower/pkg/tomlutil"
)

// Duration wraps time.Duration for TOML string unmarshaling.
type Duration = tomlutil.Duration

// ValidatorConfig holds per-validator settings.
type ValidatorConfig struct {
	Token        string   `toml:"token"`
	Permissions  []string `toml:"permissions" comment:"rpc, metrics, logs, otlp"`
	LogsMinLevel string   `toml:"logs_min_level" comment:"debug, info, warn, or error"`
}

// TokenEntry is a pre-built lookup entry mapping a token to its validator.
type TokenEntry struct {
	ValidatorName string
	Config        ValidatorConfig
}

// Config is the root watchtower configuration.
type Config struct {
	Server          ServerConfig               `toml:"server"`
	Security        SecurityConfig             `toml:"security" comment:"Rate limiting and ban settings"`
	VictoriaMetrics VictoriaMetricsConfig      `toml:"victoria_metrics" comment:"Metrics backend"`
	Loki            LokiConfig                 `toml:"loki" comment:"Log backend"`
	Validators      map[string]ValidatorConfig `toml:"validators" comment:"Each [validators.<name>] section defines a sentinel identity"`
}

type ServerConfig struct {
	ListenAddr string `toml:"listen_addr"`
}

type SecurityConfig struct {
	RateLimitRPS   float64  `toml:"rate_limit_rps"`
	RateLimitBurst int      `toml:"rate_limit_burst" comment:"must be >= 4 (sentinel sends rpc + metrics + logs + otlp concurrently), default 200"`
	BanThreshold   int      `toml:"ban_threshold" comment:"failed auth attempts before banning"`
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
		cfg.Security.RateLimitBurst = 200
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return &cfg, nil
}

// DefaultConfig returns a Config with sensible defaults and placeholders.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			ListenAddr: "0.0.0.0:8080",
		},
		Security: SecurityConfig{
			// Sized generously for debug-mode gnoland: the sentinel emits 4
			// concurrent data streams (rpc + metrics + logs + otlp), each at
			// 1-3 req/s typical; sub-min bursts add another order of magnitude.
			// A 429 is logged as watchtower_rate_limited_total{validator}.
			RateLimitRPS:   100,
			RateLimitBurst: 200,
			BanThreshold:   5,
			BanDuration:    Duration{Duration: 15 * time.Minute},
		},
		VictoriaMetrics: VictoriaMetricsConfig{
			URL: "<victoria-metrics-url>",
		},
		Loki: LokiConfig{
			URL: "<loki-url>",
		},
		Validators: map[string]ValidatorConfig{
			"my-validator": {
				Token:        "<secret-token>",
				Permissions:  []string{"rpc", "metrics", "logs", "otlp"},
				LogsMinLevel: "info",
			},
		},
	}
}

// Generate writes an annotated example TOML config to w.
func Generate(w io.Writer) error {
	cfg := DefaultConfig()
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	_, err = w.Write(data)
	return err
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
	seenTokens := make(map[string]string, len(c.Validators))
	for name, v := range c.Validators {
		if v.Token == "" {
			return fmt.Errorf("validators.%s.token is required", name)
		}
		if other, dup := seenTokens[v.Token]; dup {
			return fmt.Errorf("validators.%s and validators.%s share the same token — each validator must have a unique token", other, name)
		}
		seenTokens[v.Token] = name
	}
	return nil
}
