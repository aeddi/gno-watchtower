package self_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/sentinel/config"
	"github.com/aeddi/gno-watchtower/internal/sentinel/self"
	"github.com/aeddi/gno-watchtower/internal/sentinel/stats"
	"github.com/aeddi/gno-watchtower/pkg/logger"
	"github.com/aeddi/gno-watchtower/pkg/protocol"
)

func TestCollector_EmitsStatsPayload(t *testing.T) {
	st := stats.New()
	st.Record("rpc", 100, 100)
	st.Record("logs", 500, 120)
	st.RecordDrop("rpc")
	st.RecordRetry("logs")

	out := make(chan protocol.MetricsPayload, 2)
	cfg := config.SelfConfig{
		Enabled:        true,
		ReportInterval: config.Duration{Duration: 50 * time.Millisecond},
	}
	c := self.NewCollector(cfg, st, out, logger.Noop())

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go c.Run(ctx)

	var payload protocol.MetricsPayload
	select {
	case payload = <-out:
	case <-ctx.Done():
		t.Fatal("no payload emitted within timeout")
	}

	raw, ok := payload.Data["self_stats"]
	if !ok {
		t.Fatal("payload.Data missing self_stats key")
	}

	var byType map[string]self.TypeStats
	if err := json.Unmarshal(raw, &byType); err != nil {
		t.Fatalf("unmarshal self_stats: %v", err)
	}

	rpcStats, ok := byType["rpc"]
	if !ok {
		t.Fatal("self_stats missing rpc entry")
	}
	if rpcStats.UncompressedBytes != 100 {
		t.Errorf("rpc UncompressedBytes: got %d, want 100", rpcStats.UncompressedBytes)
	}
	if rpcStats.WireBytes != 100 {
		t.Errorf("rpc WireBytes: got %d, want 100", rpcStats.WireBytes)
	}
	if rpcStats.Drops != 1 {
		t.Errorf("rpc Drops: got %d, want 1", rpcStats.Drops)
	}

	logsStats, ok := byType["logs"]
	if !ok {
		t.Fatal("self_stats missing logs entry")
	}
	if logsStats.UncompressedBytes != 500 {
		t.Errorf("logs UncompressedBytes: got %d, want 500", logsStats.UncompressedBytes)
	}
	if logsStats.WireBytes != 120 {
		t.Errorf("logs WireBytes: got %d, want 120", logsStats.WireBytes)
	}
	if logsStats.Retries != 1 {
		t.Errorf("logs Retries: got %d, want 1", logsStats.Retries)
	}
}

func TestCollector_EmptyStatsYieldsNoPayload(t *testing.T) {
	st := stats.New()
	out := make(chan protocol.MetricsPayload, 2)
	cfg := config.SelfConfig{
		Enabled:        true,
		ReportInterval: config.Duration{Duration: 30 * time.Millisecond},
	}
	c := self.NewCollector(cfg, st, out, logger.Noop())

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	go c.Run(ctx)

	select {
	case p := <-out:
		t.Fatalf("expected no payload with empty stats, got %v", p)
	case <-ctx.Done():
	}
}
