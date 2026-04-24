package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aeddi/gno-watchtower/internal/beacon/config"
	pkgnoise "github.com/aeddi/gno-watchtower/pkg/noise"
)

func TestLoad_Valid(t *testing.T) {
	dir := t.TempDir()
	// Pre-generate a real keypair so NoiseConfig() can load it.
	kp, err := pkgnoise.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	keysDir := filepath.Join(dir, "keys")
	if err := os.MkdirAll(keysDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := pkgnoise.WriteKeypair(keysDir, kp); err != nil {
		t.Fatal(err)
	}

	tomlText := `
[server]
url = "https://watchtower.example.com/watchtower"
[beacon]
listen_addr = "0.0.0.0:8080"
keys_dir = "` + keysDir + `"
handshake_timeout = "5s"
[rpc]
rpc_url = "http://localhost:26657"
`
	p := filepath.Join(dir, "beacon.toml")
	if err := os.WriteFile(p, []byte(tomlText), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.URL != "https://watchtower.example.com/watchtower" {
		t.Errorf("URL = %q", cfg.Server.URL)
	}
	nc, err := cfg.NoiseConfig()
	if err != nil {
		t.Fatalf("NoiseConfig: %v", err)
	}
	if len(nc.Static.Private) != 32 {
		t.Error("static private key missing")
	}
}

func TestLoad_RejectsNoiseScheme(t *testing.T) {
	dir := t.TempDir()
	tomlText := `
[server]
url = "noise://watchtower:8080"
[beacon]
listen_addr = "0.0.0.0:8080"
keys_dir = "/tmp/keys"
handshake_timeout = "5s"
[rpc]
rpc_url = "http://localhost:26657"
`
	p := filepath.Join(dir, "beacon.toml")
	if err := os.WriteFile(p, []byte(tomlText), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(p)
	if err == nil || !strings.Contains(err.Error(), "http:// or https://") {
		t.Fatalf("expected http/https error, got: %v", err)
	}
}

func TestLoad_RejectsBothMetadataSources(t *testing.T) {
	dir := t.TempDir()
	tomlText := `
[server]
url = "https://watchtower:8080"
[beacon]
listen_addr = "0.0.0.0:8080"
keys_dir = "/tmp/keys"
handshake_timeout = "5s"
[rpc]
rpc_url = "http://localhost:26657"
[metadata]
config_path = "/etc/gnoland/config.toml"
config_get_cmd = "gnoland config get %s"
`
	p := filepath.Join(dir, "beacon.toml")
	if err := os.WriteFile(p, []byte(tomlText), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(p)
	if err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("expected metadata conflict error, got: %v", err)
	}
}

func TestLoad_RejectsInvalidAuthorizedKey(t *testing.T) {
	dir := t.TempDir()
	tomlText := `
[server]
url = "https://watchtower:8080"
[beacon]
listen_addr = "0.0.0.0:8080"
keys_dir = "/tmp/keys"
handshake_timeout = "5s"
authorized_keys = ["not-a-hex-key"]
[rpc]
rpc_url = "http://localhost:26657"
`
	p := filepath.Join(dir, "beacon.toml")
	if err := os.WriteFile(p, []byte(tomlText), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(p)
	if err == nil || !strings.Contains(err.Error(), "authorized_keys") {
		t.Fatalf("expected authorized_keys error, got: %v", err)
	}
}

func TestGenerate_WritesPlaceholders(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "beacon.toml")
	if err := config.Generate(p); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "<watchtower-url>") {
		t.Errorf("expected <watchtower-url> placeholder in generated config")
	}
	if !strings.Contains(string(b), "<path-to-beacon-keys-dir>") {
		t.Errorf("expected keys_dir placeholder in generated config")
	}
}

// TestLoad_AcceptsFreshGenerateConfig asserts that Load accepts the output of
// Generate verbatim. The doctor subcommand takes a config path and reports
// each placeholder as "not configured" — that requires Load to not reject
// placeholder URLs at the scheme-check stage. Matches sentinel's behaviour.
func TestLoad_AcceptsFreshGenerateConfig(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "beacon.toml")
	if err := config.Generate(p); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(p); err != nil {
		t.Errorf("Load on fresh generate-config output must succeed; got %v", err)
	}
}
