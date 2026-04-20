// internal/sentinel/metadata/collector_test.go
package metadata_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/sentinel/config"
	"github.com/aeddi/gno-watchtower/internal/sentinel/metadata"
	"github.com/aeddi/gno-watchtower/pkg/logger"
	"github.com/aeddi/gno-watchtower/pkg/protocol"
)

const sampleConfigTOML = `
moniker = "my-node"

[application]
prune_strategy = "everything"

[consensus]
peer_gossip_sleep_duration = "10ms"
timeout_commit = "3s"

[mempool]
size = 10000

[p2p]
flush_throttle_timeout = "10ms"
max_num_outbound_peers = 40
pex = true
`

func TestCollector_ConfigPath_EmitsConfigValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(sampleConfigTOML), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := config.MetadataConfig{
		Enabled:       true,
		CheckInterval: config.Duration{Duration: 10 * time.Millisecond},
		ConfigPath:    path,
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
		raw, ok := p.Data["config"]
		if !ok {
			t.Fatal("expected data[config]")
		}
		var values map[string]string
		if err := json.Unmarshal(raw, &values); err != nil {
			t.Fatalf("unmarshal config: %v", err)
		}
		wantKeys := map[string]string{
			"application.prune_strategy":           "everything",
			"consensus.peer_gossip_sleep_duration": "10ms",
			"consensus.timeout_commit":             "3s",
			"mempool.size":                         "10000",
			"p2p.flush_throttle_timeout":           "10ms",
			"p2p.max_num_outbound_peers":           "40",
			"p2p.pex":                              "true",
		}
		for k, want := range wantKeys {
			if got := values[k]; got != want {
				t.Errorf("config[%q] = %q, want %q", k, got, want)
			}
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected payload within 200ms")
	}
}

func TestReadConfigKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(sampleConfigTOML), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		key  string
		want string
	}{
		{"moniker", "my-node"},
		{"application.prune_strategy", "everything"},
		{"consensus.peer_gossip_sleep_duration", "10ms"},
		{"consensus.timeout_commit", "3s"},
		{"mempool.size", "10000"},
		{"p2p.flush_throttle_timeout", "10ms"},
		{"p2p.max_num_outbound_peers", "40"},
		{"p2p.pex", "true"},
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
