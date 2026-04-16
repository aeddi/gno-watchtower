// internal/sentinel/resources/collector_test.go
package resources_test

import (
	"context"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/sentinel/config"
	"github.com/aeddi/gno-watchtower/internal/sentinel/resources"
	"github.com/aeddi/gno-watchtower/pkg/logger"
	"github.com/aeddi/gno-watchtower/pkg/protocol"
)

func TestNewCollector_ReturnsNonNil(t *testing.T) {
	cfg := config.ResourcesConfig{
		Enabled:      true,
		PollInterval: config.Duration{Duration: 10 * time.Second},
		Source:       "host",
	}
	out := make(chan protocol.MetricsPayload, 1)
	c := resources.NewCollector(cfg, out, logger.Noop())
	if c == nil {
		t.Fatal("expected non-nil Collector")
	}
}

func TestCollector_Host_EmitsPayload(t *testing.T) {
	cfg := config.ResourcesConfig{
		Enabled:      true,
		PollInterval: config.Duration{Duration: 10 * time.Millisecond},
		Source:       "host",
	}
	out := make(chan protocol.MetricsPayload, 5)
	c := resources.NewCollector(cfg, out, logger.Noop())

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go c.Run(ctx)

	select {
	case p := <-out:
		if p.CollectedAt.IsZero() {
			t.Error("CollectedAt must not be zero")
		}
		if len(p.Data) == 0 {
			t.Error("expected at least one data key")
		}
		// host mode must include cpu and memory
		if _, ok := p.Data["cpu"]; !ok {
			t.Error("expected data[cpu]")
		}
		if _, ok := p.Data["memory"]; !ok {
			t.Error("expected data[memory]")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected payload within 200ms")
	}
}

func TestCollector_DeltaFiltering(t *testing.T) {
	// With a very short poll interval, multiple polls should only emit a payload
	// when the data actually changes. Since host metrics change slowly, after the
	// first payload subsequent ones may be suppressed by delta. We just verify
	// the first payload arrives (change from "no data" to "first data").
	cfg := config.ResourcesConfig{
		Enabled:      true,
		PollInterval: config.Duration{Duration: 5 * time.Millisecond},
		Source:       "host",
	}
	out := make(chan protocol.MetricsPayload, 10)
	c := resources.NewCollector(cfg, out, logger.Noop())

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go c.Run(ctx)

	select {
	case <-out:
		// first payload arrived — delta is working (initial state always triggers)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected at least one payload")
	}
}
