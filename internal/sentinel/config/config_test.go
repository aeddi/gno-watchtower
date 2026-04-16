package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/sentinel/config"
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

func TestLoad_LogsConfig(t *testing.T) {
	const content = `
[server]
url   = "https://example.com"
token = "tok"

[logs]
enabled        = true
source         = "docker"
container_name = "gnoland"
batch_size     = "1MB"
batch_timeout  = "5s"
min_level      = "warn"
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
	if !cfg.Logs.Enabled {
		t.Error("Logs.Enabled: want true")
	}
	if cfg.Logs.Source != "docker" {
		t.Errorf("Logs.Source: got %q, want %q", cfg.Logs.Source, "docker")
	}
	if cfg.Logs.ContainerName != "gnoland" {
		t.Errorf("Logs.ContainerName: got %q, want %q", cfg.Logs.ContainerName, "gnoland")
	}
	if int64(cfg.Logs.BatchSize) != 1024*1024 {
		t.Errorf("Logs.BatchSize: got %d, want %d", int64(cfg.Logs.BatchSize), 1024*1024)
	}
	if cfg.Logs.BatchTimeout.Duration != 5*time.Second {
		t.Errorf("Logs.BatchTimeout: got %v, want %v", cfg.Logs.BatchTimeout.Duration, 5*time.Second)
	}
	if cfg.Logs.MinLevel != "warn" {
		t.Errorf("Logs.MinLevel: got %q, want %q", cfg.Logs.MinLevel, "warn")
	}
}

func TestByteSize_UnmarshalText(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"1MB", 1024 * 1024},
		{"512KB", 512 * 1024},
		{"1GB", 1024 * 1024 * 1024},
		{"1024", 1024},
		{"1mb", 1024 * 1024}, // case-insensitive
	}
	for _, tt := range tests {
		var b config.ByteSize
		if err := b.UnmarshalText([]byte(tt.input)); err != nil {
			t.Errorf("UnmarshalText(%q): %v", tt.input, err)
			continue
		}
		if int64(b) != tt.want {
			t.Errorf("UnmarshalText(%q): got %d, want %d", tt.input, int64(b), tt.want)
		}
	}
}

func TestByteSize_UnmarshalText_Invalid(t *testing.T) {
	var b config.ByteSize
	if err := b.UnmarshalText([]byte("notanumber")); err == nil {
		t.Error("expected error for invalid byte size")
	}
}

func TestLoad_OTLPConfig(t *testing.T) {
	const content = `
[server]
url   = "https://example.com"
token = "tok"

[otlp]
enabled     = true
listen_addr = "localhost:4317"
`
	cfg := mustLoadTOML(t, content)
	if !cfg.OTLP.Enabled {
		t.Error("OTLP.Enabled: want true")
	}
	if cfg.OTLP.ListenAddr != "localhost:4317" {
		t.Errorf("OTLP.ListenAddr: got %q, want %q", cfg.OTLP.ListenAddr, "localhost:4317")
	}
}

func TestLoad_ResourcesConfig(t *testing.T) {
	const content = `
[server]
url   = "https://example.com"
token = "tok"

[resources]
enabled        = true
poll_interval  = "10s"
source         = "host"
container_name = ""
`
	cfg := mustLoadTOML(t, content)
	if !cfg.Resources.Enabled {
		t.Error("Resources.Enabled: want true")
	}
	if cfg.Resources.PollInterval.Duration != 10*time.Second {
		t.Errorf("Resources.PollInterval: got %v, want 10s", cfg.Resources.PollInterval.Duration)
	}
	if cfg.Resources.Source != "host" {
		t.Errorf("Resources.Source: got %q, want %q", cfg.Resources.Source, "host")
	}
}

func TestLoad_MetadataConfig(t *testing.T) {
	const content = `
[server]
url   = "https://example.com"
token = "tok"

[metadata]
enabled        = true
check_interval = "10m"
binary_path    = "/usr/local/bin/gnoland"
config_path    = "/etc/gnoland/config.toml"
genesis_path   = "/etc/gnoland/genesis.json"
`
	cfg := mustLoadTOML(t, content)
	if !cfg.Metadata.Enabled {
		t.Error("Metadata.Enabled: want true")
	}
	if cfg.Metadata.CheckInterval.Duration != 10*time.Minute {
		t.Errorf("Metadata.CheckInterval: got %v, want 10m", cfg.Metadata.CheckInterval.Duration)
	}
	if cfg.Metadata.BinaryPath != "/usr/local/bin/gnoland" {
		t.Errorf("Metadata.BinaryPath: got %q", cfg.Metadata.BinaryPath)
	}
	if cfg.Metadata.ConfigPath != "/etc/gnoland/config.toml" {
		t.Errorf("Metadata.ConfigPath: got %q", cfg.Metadata.ConfigPath)
	}
	if cfg.Metadata.GenesisPath != "/etc/gnoland/genesis.json" {
		t.Errorf("Metadata.GenesisPath: got %q", cfg.Metadata.GenesisPath)
	}
}

func TestLoad_EnabledCollectorRequiresServerURL(t *testing.T) {
	const content = `
[server]
url   = ""
token = ""

[rpc]
enabled       = true
poll_interval = "1s"
rpc_url       = "http://localhost:26657"
dump_consensus_state_interval = "1m"
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
		t.Error("expected error: server.url required when a collector is enabled")
	}
}

func TestLoad_AllDisabled_NoValidationError(t *testing.T) {
	const content = `
[server]
url   = ""
token = ""
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
	if err != nil {
		t.Errorf("unexpected error when all collectors disabled: %v", err)
	}
}

func TestByteSize_MarshalText(t *testing.T) {
	tests := []struct {
		input config.ByteSize
		want  string
	}{
		{config.ByteSize(1024 * 1024), "1MB"},
		{config.ByteSize(512 * 1024), "512KB"},
		{config.ByteSize(1024 * 1024 * 1024), "1GB"},
		{config.ByteSize(1024), "1KB"},
		{config.ByteSize(500), "500"},
	}
	for _, tt := range tests {
		got, err := tt.input.MarshalText()
		if err != nil {
			t.Errorf("MarshalText(%d): %v", int64(tt.input), err)
			continue
		}
		if string(got) != tt.want {
			t.Errorf("MarshalText(%d): got %q, want %q", int64(tt.input), string(got), tt.want)
		}
	}
}

func TestByteSize_RoundTrip(t *testing.T) {
	sizes := []string{"1MB", "512KB", "1GB", "1024"}
	for _, s := range sizes {
		var b config.ByteSize
		if err := b.UnmarshalText([]byte(s)); err != nil {
			t.Fatalf("UnmarshalText(%q): %v", s, err)
		}
		got, err := b.MarshalText()
		if err != nil {
			t.Fatalf("MarshalText: %v", err)
		}
		var b2 config.ByteSize
		if err := b2.UnmarshalText(got); err != nil {
			t.Fatalf("re-UnmarshalText(%q): %v", string(got), err)
		}
		if b != b2 {
			t.Errorf("round-trip %q: got %d, want %d", s, int64(b2), int64(b))
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := config.DefaultConfig()

	if cfg.Server.URL != "<watchtower-server-url>" {
		t.Errorf("Server.URL: got %q", cfg.Server.URL)
	}
	if cfg.Server.Token != "<watchtower-auth-token>" {
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
	if !cfg.Logs.Enabled {
		t.Error("Logs.Enabled: want true")
	}
	if cfg.Logs.Source != "docker" {
		t.Errorf("Logs.Source: got %q", cfg.Logs.Source)
	}
	if cfg.Logs.ContainerName != "<gnoland-container-name>" {
		t.Errorf("Logs.ContainerName: got %q", cfg.Logs.ContainerName)
	}
	if int64(cfg.Logs.BatchSize) != 1024*1024 {
		t.Errorf("Logs.BatchSize: got %d", int64(cfg.Logs.BatchSize))
	}
	if cfg.Logs.BatchTimeout.Duration != 5*time.Second {
		t.Errorf("Logs.BatchTimeout: got %v", cfg.Logs.BatchTimeout.Duration)
	}
	if cfg.Logs.MinLevel != "info" {
		t.Errorf("Logs.MinLevel: got %q", cfg.Logs.MinLevel)
	}
	if !cfg.OTLP.Enabled {
		t.Error("OTLP.Enabled: want true")
	}
	if cfg.OTLP.ListenAddr != "localhost:4317" {
		t.Errorf("OTLP.ListenAddr: got %q", cfg.OTLP.ListenAddr)
	}
	if !cfg.Resources.Enabled {
		t.Error("Resources.Enabled: want true")
	}
	if cfg.Resources.PollInterval.Duration != 10*time.Second {
		t.Errorf("Resources.PollInterval: got %v", cfg.Resources.PollInterval.Duration)
	}
	if cfg.Resources.Source != "host" {
		t.Errorf("Resources.Source: got %q", cfg.Resources.Source)
	}
	if cfg.Resources.ContainerName != "<gnoland-container-name>" {
		t.Errorf("Resources.ContainerName: got %q", cfg.Resources.ContainerName)
	}
	if !cfg.Metadata.Enabled {
		t.Error("Metadata.Enabled: want true")
	}
	if cfg.Metadata.CheckInterval.Duration != 10*time.Minute {
		t.Errorf("Metadata.CheckInterval: got %v", cfg.Metadata.CheckInterval.Duration)
	}
	if cfg.Metadata.BinaryPath != "<path-to-gnoland>" {
		t.Errorf("Metadata.BinaryPath: got %q", cfg.Metadata.BinaryPath)
	}
	if cfg.Metadata.ConfigPath != "<path-to-gnoland-config>" {
		t.Errorf("Metadata.ConfigPath: got %q", cfg.Metadata.ConfigPath)
	}
	if cfg.Metadata.GenesisPath != "<path-to-genesis-json>" {
		t.Errorf("Metadata.GenesisPath: got %q", cfg.Metadata.GenesisPath)
	}
}

// mustLoadTOML is a test helper that writes content to a temp file and loads it.
func mustLoadTOML(t *testing.T, content string) *config.Config {
	t.Helper()
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
	return cfg
}
