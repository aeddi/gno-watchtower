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

func TestNewCollector_ReturnsNonNil(t *testing.T) {
	cfg := config.MetadataConfig{
		Enabled:       true,
		CheckInterval: config.Duration{Duration: time.Minute},
	}
	out := make(chan protocol.MetricsPayload, 1)
	c := metadata.NewCollector(cfg, out, logger.Noop())
	if c == nil {
		t.Fatal("expected non-nil Collector")
	}
}

func TestCollector_BinaryPath_EmitsChecksum(t *testing.T) {
	// Write a temp file to use as the "binary".
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "gnoland")
	if err := os.WriteFile(binPath, []byte("fake binary content"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.MetadataConfig{
		Enabled:       true,
		CheckInterval: config.Duration{Duration: 10 * time.Millisecond},
		BinaryPath:    binPath,
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
		if _, ok := p.Data["binary_checksum"]; !ok {
			t.Error("expected data[binary_checksum]")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected payload within 200ms")
	}
}

func TestCollector_ConflictDetection_SkipsItem(t *testing.T) {
	// When both path and cmd are set for the same item, neither should be collected.
	tmpDir := t.TempDir()
	genPath := filepath.Join(tmpDir, "genesis.json")
	if err := os.WriteFile(genPath, []byte(`{"chain_id":"test"}`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.MetadataConfig{
		Enabled:           true,
		CheckInterval:     config.Duration{Duration: 10 * time.Millisecond},
		BinaryPath:        "/usr/local/bin/gnoland",
		BinaryChecksumCmd: "echo fakehash", // CONFLICT: both set
		GenesisPath:       genPath,         // only path set — valid
	}
	out := make(chan protocol.MetricsPayload, 5)
	c := metadata.NewCollector(cfg, out, logger.Noop())

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go c.Run(ctx)

	select {
	case p := <-out:
		if _, ok := p.Data["binary_checksum"]; ok {
			t.Error("binary_checksum must not be collected when path and cmd both set")
		}
		if _, ok := p.Data["genesis_checksum"]; !ok {
			t.Error("genesis_checksum must be collected (only path set)")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected payload within 200ms")
	}
}
