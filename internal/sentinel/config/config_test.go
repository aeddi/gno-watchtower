package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/gnolang/val-companion/internal/sentinel/config"
)

func TestLoad(t *testing.T) {
	content := `
[server]
url   = "https://monitoring.example.com"
token = "test-token"

[rpc]
enabled       = true
poll_interval = "3s"
rpc_url       = "http://localhost:26657"
dump_consensus_state_interval = "30s"
`
	f, err := os.CreateTemp("", "sentinel-config-*.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()

	cfg, err := config.Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Server.URL != "https://monitoring.example.com" {
		t.Errorf("Server.URL: got %q", cfg.Server.URL)
	}
	if cfg.Server.Token != "test-token" {
		t.Errorf("Server.Token: got %q", cfg.Server.Token)
	}
	if !cfg.RPC.Enabled {
		t.Error("RPC.Enabled: want true")
	}
	if cfg.RPC.PollInterval.Duration != 3*time.Second {
		t.Errorf("RPC.PollInterval: got %v", cfg.RPC.PollInterval.Duration)
	}
	if cfg.RPC.RPCURL != "http://localhost:26657" {
		t.Errorf("RPC.RPCURL: got %q", cfg.RPC.RPCURL)
	}
	if cfg.RPC.DumpConsensusStateInterval.Duration != 30*time.Second {
		t.Errorf("RPC.DumpConsensusStateInterval: got %v", cfg.RPC.DumpConsensusStateInterval.Duration)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := config.Load("/nonexistent/path.toml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoad_InvalidDuration(t *testing.T) {
	content := `
[rpc]
poll_interval = "notaduration"
`
	f, err := os.CreateTemp("", "sentinel-config-*.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()

	_, err = config.Load(f.Name())
	if err == nil {
		t.Fatal("expected error for invalid duration")
	}
}

func TestExample_IsValidTOML(t *testing.T) {
	f, err := os.CreateTemp("", "sentinel-example-*.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(config.Example); err != nil {
		t.Fatal(err)
	}
	f.Close()

	if _, err := config.Load(f.Name()); err != nil {
		t.Fatalf("Example constant is not valid TOML: %v", err)
	}
}
