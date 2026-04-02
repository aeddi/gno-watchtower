// internal/sentinel/metadata/collector.go
package metadata

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/fsnotify/fsnotify"

	"github.com/gnolang/val-companion/internal/sentinel/config"
	"github.com/gnolang/val-companion/internal/sentinel/delta"
	"github.com/gnolang/val-companion/pkg/protocol"
)

// ConfigKeys is the list of gnoland config keys collected by the metadata collector.
var ConfigKeys = []string{
	"moniker",
	"db_backend",
	"p2p.laddr",
	"p2p.persistent_peers",
	"rpc.laddr",
	"telemetry.enabled",
	"telemetry.exporter_endpoint",
}

// Collector gathers binary/genesis checksums and gnoland config key values.
// Each item uses either a file path (direct sha256 + fsnotify change watch) or
// a shell command (polled on check_interval). Setting both for the same item is
// an error — the item is skipped and an error is logged.
type Collector struct {
	cfg   config.MetadataConfig
	delta *delta.Delta
	out   chan<- protocol.MetricsPayload
	log   *slog.Logger
}

// NewCollector creates a metadata Collector.
func NewCollector(cfg config.MetadataConfig, out chan<- protocol.MetricsPayload, log *slog.Logger) *Collector {
	return &Collector{
		cfg:   cfg,
		delta: delta.NewDelta(),
		out:   out,
		log:   log.With("component", "metadata_collector"),
	}
}

// Run performs an initial collection, then watches file paths for changes and
// polls on check_interval for cmd-mode items. Blocks until ctx is cancelled.
func (c *Collector) Run(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	defer watcher.Close()

	for _, path := range c.watchedPaths() {
		if err := watcher.Add(path); err != nil {
			c.log.Warn("watch path failed", "path", path, "err", err)
		}
	}

	c.collectAndSend(ctx)

	ticker := time.NewTicker(c.cfg.CheckInterval.Duration)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			c.collectAndSend(ctx)
		case event, ok := <-watcher.Events:
			if !ok {
				c.log.Warn("watcher events channel closed unexpectedly")
				return nil
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				c.log.Debug("file changed", "path", event.Name)
				c.collectAndSend(ctx)
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				c.log.Warn("watcher errors channel closed unexpectedly")
				return nil
			}
			c.log.Warn("watcher error", "err", err)
		}
	}
}

// watchedPaths returns the file paths being watched (path-mode items only, conflict-free).
func (c *Collector) watchedPaths() []string {
	var paths []string
	if c.cfg.BinaryPath != "" && c.cfg.BinaryChecksumCmd == "" {
		paths = append(paths, c.cfg.BinaryPath)
	}
	if c.cfg.GenesisPath != "" && c.cfg.GenesisChecksumCmd == "" {
		paths = append(paths, c.cfg.GenesisPath)
	}
	if c.cfg.ConfigPath != "" && c.cfg.ConfigGetCmd == "" {
		paths = append(paths, c.cfg.ConfigPath)
	}
	return paths
}

func (c *Collector) collectAndSend(ctx context.Context) {
	payload := protocol.MetricsPayload{
		CollectedAt: time.Now().UTC(),
		Data:        make(map[string]json.RawMessage),
	}
	c.collectBinary(payload.Data)
	c.collectGenesis(payload.Data)
	c.collectConfig(payload.Data)
	if len(payload.Data) == 0 {
		return
	}
	select {
	case c.out <- payload:
	case <-ctx.Done():
	}
}

func (c *Collector) collectBinary(data map[string]json.RawMessage) {
	c.collectChecksum(data, "binary_checksum", "binary",
		c.cfg.BinaryPath, c.cfg.BinaryChecksumCmd,
		"metadata conflict: binary_path and binary_checksum_cmd both set — skipping binary checksum")
}

func (c *Collector) collectGenesis(data map[string]json.RawMessage) {
	c.collectChecksum(data, "genesis_checksum", "genesis",
		c.cfg.GenesisPath, c.cfg.GenesisChecksumCmd,
		"metadata conflict: genesis_path and genesis_checksum_cmd both set — skipping genesis checksum")
}

func (c *Collector) collectChecksum(data map[string]json.RawMessage, dataKey, name, path, cmd, conflictMsg string) {
	if path != "" && cmd != "" {
		c.log.Error(conflictMsg)
		return
	}
	var checksum string
	var err error
	switch {
	case path != "":
		checksum, err = SHA256File(path)
	case cmd != "":
		checksum, err = RunCmd(cmd)
	default:
		return
	}
	if err != nil {
		c.log.Warn(name+" checksum error", "err", err)
		return
	}
	b, err := json.Marshal(checksum)
	if err != nil {
		c.log.Warn(name+" checksum marshal error", "err", err)
		return
	}
	if c.delta.Changed(dataKey, b) {
		data[dataKey] = b
	}
}

func (c *Collector) collectConfig(data map[string]json.RawMessage) {
	if c.cfg.ConfigPath != "" && c.cfg.ConfigGetCmd != "" {
		c.log.Error("metadata conflict: config_path and config_get_cmd both set — skipping config keys")
		return
	}
	if c.cfg.ConfigPath == "" && c.cfg.ConfigGetCmd == "" {
		return
	}
	values := make(map[string]string)
	for _, key := range ConfigKeys {
		var val string
		var err error
		if c.cfg.ConfigPath != "" {
			val, err = ReadConfigKey(c.cfg.ConfigPath, key)
		} else {
			cmd := strings.ReplaceAll(c.cfg.ConfigGetCmd, "%s", key)
			val, err = RunCmd(cmd)
		}
		if err != nil {
			c.log.Warn("config key error", "key", key, "err", err)
			continue
		}
		values[key] = val
	}
	if len(values) == 0 {
		return
	}
	b, err := json.Marshal(values)
	if err != nil {
		c.log.Warn("config marshal error", "err", err)
		return
	}
	if c.delta.Changed("config", b) {
		data["config"] = b
	}
}

// SHA256File returns the hex-encoded SHA-256 checksum of the file at path.
func SHA256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// RunCmd runs cmd via sh -c and returns the trimmed stdout.
func RunCmd(cmd string) (string, error) {
	out, err := exec.Command("sh", "-c", cmd).Output()
	if err != nil {
		return "", fmt.Errorf("run %q: %w", cmd, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ReadConfigKey reads a dot-separated key from a TOML config file.
// For example "p2p.laddr" navigates to the [p2p] section and returns the laddr value.
// Single-segment keys (e.g. "moniker") are read from the top-level table.
func ReadConfigKey(configPath, key string) (string, error) {
	b, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", configPath, err)
	}
	var root map[string]any
	if _, err := toml.Decode(string(b), &root); err != nil {
		return "", fmt.Errorf("parse %s: %w", configPath, err)
	}
	parts := strings.Split(key, ".")
	node := any(root)
	for _, part := range parts[:len(parts)-1] {
		m, ok := node.(map[string]any)
		if !ok {
			return "", fmt.Errorf("key %q not found in %s", key, configPath)
		}
		v, ok := m[part]
		if !ok {
			return "", fmt.Errorf("key %q not found in %s", key, configPath)
		}
		node = v
	}
	m, ok := node.(map[string]any)
	if !ok {
		return "", fmt.Errorf("key %q not found in %s", key, configPath)
	}
	v, ok := m[parts[len(parts)-1]]
	if !ok {
		return "", fmt.Errorf("key %q not found in %s", key, configPath)
	}
	return fmt.Sprintf("%v", v), nil
}
