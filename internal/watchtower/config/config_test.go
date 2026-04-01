package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gnolang/val-companion/internal/watchtower/config"
)

const exampleTOML = `
[server]
listen_addr = "127.0.0.1:8080"

[security]
rate_limit_rps = 10
ban_threshold  = 5
ban_duration   = "15m"

[victoria_metrics]
url = "http://victoria-metrics:8428"

[loki]
url = "http://loki:3100"

[validators.val-01]
token         = "secret-token-1"
permissions   = ["rpc", "metrics", "logs", "otlp"]
logs_min_level = "info"

[validators.val-02]
token         = "secret-token-2"
permissions   = ["rpc", "metrics"]
logs_min_level = "warn"
`

func TestLoad_ParsesAllFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(exampleTOML), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Server.ListenAddr != "127.0.0.1:8080" {
		t.Errorf("listen_addr: got %q", cfg.Server.ListenAddr)
	}
	if cfg.Security.RateLimitRPS != 10 {
		t.Errorf("rate_limit_rps: got %v", cfg.Security.RateLimitRPS)
	}
	if cfg.Security.BanThreshold != 5 {
		t.Errorf("ban_threshold: got %v", cfg.Security.BanThreshold)
	}
	if cfg.Security.BanDuration.Duration != 15*time.Minute {
		t.Errorf("ban_duration: got %v", cfg.Security.BanDuration.Duration)
	}
	if cfg.VictoriaMetrics.URL != "http://victoria-metrics:8428" {
		t.Errorf("vm url: got %q", cfg.VictoriaMetrics.URL)
	}
	if cfg.Loki.URL != "http://loki:3100" {
		t.Errorf("loki url: got %q", cfg.Loki.URL)
	}

	v01, ok := cfg.Validators["val-01"]
	if !ok {
		t.Fatal("val-01 missing")
	}
	if v01.Token != "secret-token-1" {
		t.Errorf("val-01 token: got %q", v01.Token)
	}
	wantPerms := []string{"rpc", "metrics", "logs", "otlp"}
	if len(v01.Permissions) != len(wantPerms) {
		t.Errorf("val-01 permissions length: want %d, got %d", len(wantPerms), len(v01.Permissions))
	} else {
		for i, p := range wantPerms {
			if v01.Permissions[i] != p {
				t.Errorf("val-01 permissions[%d]: want %q, got %q", i, p, v01.Permissions[i])
			}
		}
	}
	if v01.LogsMinLevel != "info" {
		t.Errorf("val-01 logs_min_level: got %q", v01.LogsMinLevel)
	}
}

func TestLoad_BuildsTokenIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(exampleTOML), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	entry, ok := cfg.TokenIndex["secret-token-1"]
	if !ok {
		t.Fatal("token not found in index")
	}
	if entry.ValidatorName != "val-01" {
		t.Errorf("validator name: got %q", entry.ValidatorName)
	}
	if entry.Config.LogsMinLevel != "info" {
		t.Errorf("logs_min_level: got %q", entry.Config.LogsMinLevel)
	}
	if _, ok := cfg.TokenIndex["secret-token-2"]; !ok {
		t.Error("secret-token-2 not found in token index")
	}
}

func TestLoad_MissingFile_ReturnsError(t *testing.T) {
	_, err := config.Load("/nonexistent/config.toml")
	if err == nil {
		t.Error("expected error for missing file")
	}
	if err != nil && err.Error() == "" {
		t.Error("error message must be non-empty")
	}
}
