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

	"github.com/fsnotify/fsnotify"

	"github.com/gnolang/val-companion/internal/sentinel/config"
	"github.com/gnolang/val-companion/internal/sentinel/delta"
	"github.com/gnolang/val-companion/pkg/protocol"
)

// configKeys is the list of gnoland config keys collected by the metadata collector.
var configKeys = []string{
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
				return nil
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				c.log.Debug("file changed", "path", event.Name)
				c.collectAndSend(ctx)
			}
		case err, ok := <-watcher.Errors:
			if !ok {
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
	if c.cfg.BinaryPath != "" && c.cfg.BinaryChecksumCmd != "" {
		c.log.Error("metadata conflict: binary_path and binary_checksum_cmd both set — skipping binary checksum")
		return
	}
	var checksum string
	var err error
	switch {
	case c.cfg.BinaryPath != "":
		checksum, err = sha256File(c.cfg.BinaryPath)
	case c.cfg.BinaryChecksumCmd != "":
		checksum, err = runCmd(c.cfg.BinaryChecksumCmd)
	default:
		return
	}
	if err != nil {
		c.log.Warn("binary checksum error", "err", err)
		return
	}
	if b, err := json.Marshal(checksum); err == nil && c.delta.Changed("binary_checksum", b) {
		data["binary_checksum"] = b
	}
}

func (c *Collector) collectGenesis(data map[string]json.RawMessage) {
	if c.cfg.GenesisPath != "" && c.cfg.GenesisChecksumCmd != "" {
		c.log.Error("metadata conflict: genesis_path and genesis_checksum_cmd both set — skipping genesis checksum")
		return
	}
	var checksum string
	var err error
	switch {
	case c.cfg.GenesisPath != "":
		checksum, err = sha256File(c.cfg.GenesisPath)
	case c.cfg.GenesisChecksumCmd != "":
		checksum, err = runCmd(c.cfg.GenesisChecksumCmd)
	default:
		return
	}
	if err != nil {
		c.log.Warn("genesis checksum error", "err", err)
		return
	}
	if b, err := json.Marshal(checksum); err == nil && c.delta.Changed("genesis_checksum", b) {
		data["genesis_checksum"] = b
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
	for _, key := range configKeys {
		var val string
		var err error
		if c.cfg.ConfigPath != "" {
			val, err = readConfigKey(c.cfg.ConfigPath, key)
		} else {
			cmd := strings.ReplaceAll(c.cfg.ConfigGetCmd, "%s", key)
			val, err = runCmd(cmd)
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
	if b, err := json.Marshal(values); err == nil && c.delta.Changed("config", b) {
		data["config"] = b
	}
}

// sha256File returns the hex-encoded SHA-256 checksum of the file at path.
func sha256File(path string) (string, error) {
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

// runCmd runs cmd via sh -c and returns the trimmed stdout.
func runCmd(cmd string) (string, error) {
	out, err := exec.Command("sh", "-c", cmd).Output()
	if err != nil {
		return "", fmt.Errorf("run %q: %w", cmd, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// readConfigKey reads a key from a TOML config file by scanning for `leafKey = value` lines.
// Key is a dot-separated path (e.g. "p2p.laddr"); only the leaf segment is matched.
func readConfigKey(configPath, key string) (string, error) {
	b, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", configPath, err)
	}
	leaf := key
	if idx := strings.LastIndex(key, "."); idx >= 0 {
		leaf = key[idx+1:]
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) != leaf {
			continue
		}
		val := strings.TrimSpace(parts[1])
		if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
			val = val[1 : len(val)-1]
		}
		return val, nil
	}
	return "", fmt.Errorf("key %q not found in %s", key, configPath)
}
