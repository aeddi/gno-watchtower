package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/watchtower/config"
)

const exampleTOML = `
[server]
listen_addr = "127.0.0.1:8080"

[security]
rate_limit_rps  = 10
rate_limit_burst = 20
ban_threshold    = 5
ban_duration     = "15m"

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
	if cfg.Security.RateLimitBurst != 20 {
		t.Errorf("rate_limit_burst: got %v", cfg.Security.RateLimitBurst)
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

func TestLoad_MissingFile_ReturnsError(t *testing.T) {
	_, err := config.Load("/nonexistent/config.toml")
	if err == nil {
		t.Error("expected error for missing file")
	}
	if err != nil && err.Error() == "" {
		t.Error("error message must be non-empty")
	}
}

func TestLoad_RateLimitBurstDefault(t *testing.T) {
	// TOML with no rate_limit_burst — Load must default it to 200.
	const content = `
[server]
listen_addr = "127.0.0.1:8080"

[security]
rate_limit_rps = 5
ban_threshold   = 3
ban_duration    = "5m"

[victoria_metrics]
url = "http://vm:8428"

[loki]
url = "http://loki:3100"
`
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Security.RateLimitBurst != 200 {
		t.Errorf("RateLimitBurst default: got %d, want 200", cfg.Security.RateLimitBurst)
	}
}

func TestLoad_MissingListenAddr_ReturnsError(t *testing.T) {
	const content = `
[security]
rate_limit_rps = 5
ban_threshold  = 3
ban_duration   = "5m"
[victoria_metrics]
url = "http://vm:8428"
[loki]
url = "http://loki:3100"
`
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(path)
	if err == nil {
		t.Error("expected error for missing listen_addr")
	}
}

func TestLoad_ZeroRateLimitRPS_ReturnsError(t *testing.T) {
	const content = `
[server]
listen_addr = "127.0.0.1:8080"
[security]
ban_threshold = 3
ban_duration  = "5m"
[victoria_metrics]
url = "http://vm:8428"
[loki]
url = "http://loki:3100"
`
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(path)
	if err == nil {
		t.Error("expected error for zero rate_limit_rps")
	}
}

func TestLoad_MissingVMURL_ReturnsError(t *testing.T) {
	const content = `
[server]
listen_addr = "127.0.0.1:8080"
[security]
rate_limit_rps = 5
ban_threshold  = 3
ban_duration   = "5m"
[loki]
url = "http://loki:3100"
`
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(path)
	if err == nil {
		t.Error("expected error for missing victoria_metrics.url")
	}
}

func TestLoad_DuplicateValidatorToken_ReturnsError(t *testing.T) {
	const content = `
[server]
listen_addr = "127.0.0.1:8080"
[security]
rate_limit_rps = 5
ban_threshold  = 3
ban_duration   = "5m"
[victoria_metrics]
url = "http://vm:8428"
[loki]
url = "http://loki:3100"
[validators.val-01]
token = "shared-token"
permissions = ["rpc"]
[validators.val-02]
token = "shared-token"
permissions = ["rpc"]
`
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for duplicate validator tokens")
	}
}

func TestLoad_InvalidPermission_ReturnsError(t *testing.T) {
	const content = `
[server]
listen_addr = "127.0.0.1:8080"
[security]
rate_limit_rps = 5
ban_threshold  = 3
ban_duration   = "5m"
[victoria_metrics]
url = "http://vm:8428"
[loki]
url = "http://loki:3100"
[validators.val-01]
token = "secret"
permissions = ["rpc", "banana"]
`
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for unknown permission")
	}
}

func TestLoad_EmptyPermissions_OK(t *testing.T) {
	// An empty permissions list is legal (collectors-enforcement at request
	// time) — validate() must not reject it.
	const content = `
[server]
listen_addr = "127.0.0.1:8080"
[security]
rate_limit_rps = 5
ban_threshold  = 3
ban_duration   = "5m"
[victoria_metrics]
url = "http://vm:8428"
[loki]
url = "http://loki:3100"
[validators.val-01]
token = "secret"
permissions = []
`
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(path); err != nil {
		t.Fatalf("empty permissions must be accepted: %v", err)
	}
}

func TestLoad_RateLimitBurstBelowMin_ReturnsError(t *testing.T) {
	const content = `
[server]
listen_addr = "127.0.0.1:8080"
[security]
rate_limit_rps   = 5
rate_limit_burst = 3
ban_threshold    = 3
ban_duration     = "5m"
[victoria_metrics]
url = "http://vm:8428"
[loki]
url = "http://loki:3100"
`
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for rate_limit_burst below minimum")
	}
}

func TestLoad_EmptyValidatorToken_ReturnsError(t *testing.T) {
	const content = `
[server]
listen_addr = "127.0.0.1:8080"
[security]
rate_limit_rps = 5
ban_threshold  = 3
ban_duration   = "5m"
[victoria_metrics]
url = "http://vm:8428"
[loki]
url = "http://loki:3100"
[validators.val-01]
token = ""
permissions = ["rpc"]
`
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(path)
	if err == nil {
		t.Error("expected error for empty validator token")
	}
}
