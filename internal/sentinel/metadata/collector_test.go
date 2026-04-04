// internal/sentinel/metadata/collector_test.go
package metadata_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gnolang/val-companion/internal/sentinel/config"
	"github.com/gnolang/val-companion/internal/sentinel/metadata"
	"github.com/gnolang/val-companion/pkg/logger"
	"github.com/gnolang/val-companion/pkg/protocol"
)

func TestCollector_BinaryVersionCmd_EmitsVersion(t *testing.T) {
	cfg := config.MetadataConfig{
		Enabled:          true,
		CheckInterval:    config.Duration{Duration: 10 * time.Millisecond},
		BinaryVersionCmd: "echo v1.2.3",
	}
	out := make(chan protocol.MetricsPayload, 5)
	c := metadata.NewCollector(cfg, out, logger.Noop())

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go c.Run(ctx)

	select {
	case p := <-out:
		if p.CollectedAt.IsZero() {
			t.Error("CollectedAt must not be zero")
		}
		raw, ok := p.Data["binary_version"]
		if !ok {
			t.Fatal("expected data[binary_version]")
		}
		if string(raw) != `"v1.2.3"` {
			t.Errorf("binary_version: got %s, want %q", raw, "v1.2.3")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected payload within 200ms")
	}
}

func TestCollector_ConflictDetection_SkipsItem(t *testing.T) {
	tmpDir := t.TempDir()
	genPath := filepath.Join(tmpDir, "genesis.json")
	if err := os.WriteFile(genPath, []byte(`{"chain_id":"test"}`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.MetadataConfig{
		Enabled:          true,
		CheckInterval:    config.Duration{Duration: 10 * time.Millisecond},
		BinaryPath:       "/usr/local/bin/gnoland",
		BinaryVersionCmd: "echo fakehash", // CONFLICT: both set
		GenesisPath:      genPath,
	}
	out := make(chan protocol.MetricsPayload, 5)
	c := metadata.NewCollector(cfg, out, logger.Noop())

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go c.Run(ctx)

	select {
	case p := <-out:
		if _, ok := p.Data["binary_version"]; ok {
			t.Error("binary_version must not be collected when path and cmd both set")
		}
		if _, ok := p.Data["genesis_checksum"]; !ok {
			t.Error("genesis_checksum must be collected (only path set)")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected payload within 200ms")
	}
}

func TestReadConfigKey(t *testing.T) {
	const content = `
moniker = "my-node"
db_backend = "goleveldb"

[p2p]
laddr = "tcp://0.0.0.0:26656"
persistent_peers = ""

[telemetry]
enabled = true
exporter_endpoint = "http://localhost:4317"
`
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		key  string
		want string
	}{
		{"moniker", "my-node"},
		{"db_backend", "goleveldb"},
		{"p2p.laddr", "tcp://0.0.0.0:26656"},
		{"p2p.persistent_peers", ""},
		{"telemetry.enabled", "true"},
		{"telemetry.exporter_endpoint", "http://localhost:4317"},
	}
	for _, tt := range tests {
		got, err := metadata.ReadConfigKey(path, tt.key)
		if err != nil {
			t.Errorf("ReadConfigKey(%q): unexpected error: %v", tt.key, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ReadConfigKey(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}

	if _, err := metadata.ReadConfigKey(path, "nonexistent"); err == nil {
		t.Error("ReadConfigKey(nonexistent): expected error, got nil")
	}
	if _, err := metadata.ReadConfigKey(path, "p2p.nonexistent"); err == nil {
		t.Error("ReadConfigKey(p2p.nonexistent): expected error, got nil")
	}
	if _, err := metadata.ReadConfigKey("/nonexistent/file.toml", "moniker"); err == nil {
		t.Error("ReadConfigKey(missing file): expected error, got nil")
	}
}
