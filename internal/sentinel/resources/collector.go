// internal/sentinel/resources/collector.go
package resources

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	dockerclient "github.com/docker/docker/client"
	gopsutilcpu "github.com/shirou/gopsutil/v3/cpu"
	gopsutildisk "github.com/shirou/gopsutil/v3/disk"
	gopsutilmem "github.com/shirou/gopsutil/v3/mem"
	gopsutilnet "github.com/shirou/gopsutil/v3/net"

	"github.com/aeddi/gno-watchtower/internal/sentinel/config"
	"github.com/aeddi/gno-watchtower/internal/sentinel/delta"
	"github.com/aeddi/gno-watchtower/pkg/protocol"
)

// Collector polls host and/or container resource metrics on a configurable interval.
// Only changed values (by hash) are included in each MetricsPayload (delta-filtered).
type Collector struct {
	cfg       config.ResourcesConfig
	delta     *delta.Delta
	out       chan<- protocol.MetricsPayload
	log       *slog.Logger
	dockerCli *dockerclient.Client // lazily initialized on first docker poll
}

// NewCollector creates a resource Collector.
func NewCollector(cfg config.ResourcesConfig, out chan<- protocol.MetricsPayload, log *slog.Logger) *Collector {
	return &Collector{
		cfg:   cfg,
		delta: delta.NewDelta(),
		out:   out,
		log:   log.With("component", "resource_collector"),
	}
}

// Run polls resources on the configured interval until ctx is cancelled.
func (c *Collector) Run(ctx context.Context) error {
	ticker := time.NewTicker(c.cfg.PollInterval.Duration)
	defer ticker.Stop()
	defer func() {
		if c.dockerCli != nil {
			c.dockerCli.Close()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := c.collect(ctx); err != nil && ctx.Err() == nil {
				c.log.Error("collect failed", "err", err)
			}
		}
	}
}

func (c *Collector) collect(ctx context.Context) error {
	payload := protocol.MetricsPayload{
		CollectedAt: time.Now().UTC(),
		Data:        make(map[string]json.RawMessage),
	}

	if c.cfg.Source == "host" || c.cfg.Source == "both" {
		c.collectHost(ctx, payload.Data)
	}
	if c.cfg.Source == "docker" || c.cfg.Source == "both" {
		c.collectDocker(ctx, payload.Data)
	}

	if len(payload.Data) == 0 {
		return nil
	}

	select {
	case c.out <- payload:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func (c *Collector) collectHost(ctx context.Context, data map[string]json.RawMessage) {
	if percents, err := gopsutilcpu.PercentWithContext(ctx, 0, false); err == nil {
		if b, err := json.Marshal(percents); err != nil {
			c.log.Warn("cpu marshal error", "err", err)
		} else if c.delta.Changed("cpu", b) {
			data["cpu"] = b
		}
	} else {
		c.log.Warn("cpu stats error", "err", err)
	}

	if vm, err := gopsutilmem.VirtualMemoryWithContext(ctx); err == nil {
		if b, err := json.Marshal(vm); err != nil {
			c.log.Warn("memory marshal error", "err", err)
		} else if c.delta.Changed("memory", b) {
			data["memory"] = b
		}
	} else {
		c.log.Warn("memory stats error", "err", err)
	}

	if usage, err := gopsutildisk.UsageWithContext(ctx, "/"); err == nil {
		if b, err := json.Marshal(usage); err != nil {
			c.log.Warn("disk marshal error", "err", err)
		} else if c.delta.Changed("disk", b) {
			data["disk"] = b
		}
	} else {
		c.log.Warn("disk stats error", "err", err)
	}

	if counters, err := gopsutilnet.IOCountersWithContext(ctx, false); err == nil {
		if b, err := json.Marshal(counters); err != nil {
			c.log.Warn("network marshal error", "err", err)
		} else if c.delta.Changed("network", b) {
			data["network"] = b
		}
	} else {
		c.log.Warn("network stats error", "err", err)
	}
}

func (c *Collector) collectDocker(ctx context.Context, data map[string]json.RawMessage) {
	if c.dockerCli == nil {
		cli, err := dockerclient.NewClientWithOpts(
			dockerclient.FromEnv,
			dockerclient.WithAPIVersionNegotiation(),
		)
		if err != nil {
			c.log.Warn("docker client error", "container", c.cfg.ContainerName, "err", err)
			return
		}
		c.dockerCli = cli
	}
	b, err := containerStatsFromClient(ctx, c.dockerCli, c.cfg.ContainerName)
	if err != nil {
		c.log.Warn("docker stats error", "container", c.cfg.ContainerName, "err", err)
		return
	}
	if c.delta.Changed("container", b) {
		data["container"] = b
	}
}
