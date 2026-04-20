// internal/sentinel/metadata/collector.go
package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	toml "github.com/pelletier/go-toml/v2"

	"github.com/aeddi/gno-watchtower/internal/sentinel/config"
	"github.com/aeddi/gno-watchtower/internal/sentinel/delta"
	"github.com/aeddi/gno-watchtower/pkg/protocol"
)

// ConfigKeys is the list of gnoland config keys collected by the metadata
// collector. Every key here is one we cannot get from /status or /genesis; see
// docs/data-collected.md for the source-picking rationale.
var ConfigKeys = []string{
	"application.prune_strategy",
	"consensus.peer_gossip_sleep_duration",
	"consensus.timeout_commit",
	"mempool.size",
	"p2p.flush_throttle_timeout",
	"p2p.max_num_outbound_peers",
	"p2p.pex",
}

// Collector gathers gnoland config key values. Binary version and genesis info
// are available from /status and /genesis RPC endpoints — the RPC forwarder
// handles those — so the metadata collector's only job is the config keys that
// aren't exposed via RPC (see ConfigKeys for the list).
//
// Config uses either a file path or a shell command. Setting both is an error.
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

// Run performs an initial collection, then watches the config file for changes
// (file mode) and polls on check_interval (cmd mode). Blocks until ctx is cancelled.
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

// watchedPaths returns the config file path being watched (file-mode only).
func (c *Collector) watchedPaths() []string {
	if c.cfg.ConfigPath != "" && c.cfg.ConfigGetCmd == "" {
		return []string{c.cfg.ConfigPath}
	}
	return nil
}

func (c *Collector) collectAndSend(ctx context.Context) {
	payload := protocol.MetricsPayload{
		CollectedAt: time.Now().UTC(),
		Data:        make(map[string]json.RawMessage),
	}
	c.collectConfig(ctx, payload.Data)
	if len(payload.Data) == 0 {
		return
	}
	select {
	case c.out <- payload:
	case <-ctx.Done():
	}
}

func (c *Collector) collectConfig(ctx context.Context, data map[string]json.RawMessage) {
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
			val, err = RunCmd(ctx, cmd)
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

// RunCmd runs cmd via sh -c and returns the trimmed stdout. The command is
// killed if ctx is cancelled before it completes.
func RunCmd(ctx context.Context, cmd string) (string, error) {
	out, err := exec.CommandContext(ctx, "sh", "-c", cmd).Output()
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
	if err := toml.Unmarshal(b, &root); err != nil {
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
