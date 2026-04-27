package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scribe.toml")
	if err := os.WriteFile(path, []byte(`
[server]
listen_addr = "0.0.0.0:8090"
[cluster]
id = "c1"
[storage]
db_path = "/tmp/x.duckdb"
[sources.victoria_metrics]
url = "http://vm:8428"
[sources.loki]
url = "http://loki:3100"
[ingest.fast]
interval = "3s"
queries = ["a","b"]
[ingest.slow]
interval = "60s"
queries = []
[ingest.logs]
streams = []
overlap_window = "5s"
[writer]
batch_size = 100
batch_window = "250ms"
[anchors]
interval = "1h"
[retention]
hot_window = "30d"
warm_window = "365d"
warm_bucket = "1m"
prune_after = "365d"
compact_at = "03:00"
[backfill]
chunk_size = "1h"
default_lookback = "30d"
resume_stale_after = "5m"
[sse]
slow_subscriber_timeout = "5s"
max_subscribers = 32
[logging]
level = "info"
format = "json"
`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Cluster.ID != "c1" {
		t.Errorf("cluster.id = %q", c.Cluster.ID)
	}
	if c.Ingest.Fast.Interval.Std() != 3*time.Second {
		t.Errorf("interval = %v", c.Ingest.Fast.Interval.Std())
	}
	if c.Retention.HotWindow.Std() != 30*24*time.Hour {
		t.Errorf("hot_window = %v want 30d", c.Retention.HotWindow.Std())
	}
}

func TestAnalysisConfigParsesRulesAndOverlay(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "scribe.toml")
	body := `
[server]
listen_addr = "127.0.0.1:0"

[cluster]
id = "c1"

[storage]
db_path = "/tmp/x.db"

[sources.victoria_metrics]
url = "http://vm"

[sources.loki]
url = "http://loki"

[ingest.fast]
interval = "1s"
queries = ["up"]

[ingest.slow]
interval = "30s"
queries = ["up"]

[ingest.logs]
streams = ['{validator=~".+"}']
overlap_window = "5s"

[writer]
batch_size = 100
batch_window = "250ms"

[anchors]
interval = "1h"

[retention]
hot_window = "24h"
warm_bucket = "30s"
prune_after = "30d"
compact_at = "03:00"
compact_jitter = "30m"

[backfill]
chunk_size = "1h"
resume_stale_after = "1m"

[sse]
slow_drop_threshold = 8

[logging]
level = "info"
format = "json"

[analysis]
enabled = true
disabled = ["diagnostic.consensus_slow_v1"]

[analysis.rules."diagnostic.bft_at_risk_v1"]
voting_power_threshold_pct = 33.33

[analysis.rules."diagnostic.consensus_stuck_v1"]
threshold_seconds = 60
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Analysis.Enabled {
		t.Errorf("Analysis.Enabled = false, want true")
	}
	if got := cfg.Analysis.Disabled; len(got) != 1 || got[0] != "diagnostic.consensus_slow_v1" {
		t.Errorf("Analysis.Disabled = %v", got)
	}
	if cfg.Analysis.Rules == nil {
		t.Fatalf("Analysis.Rules nil")
	}
	bft := cfg.Analysis.Rules["diagnostic.bft_at_risk_v1"]
	if v, ok := bft["voting_power_threshold_pct"].(float64); !ok || v != 33.33 {
		t.Errorf("bft_at_risk_v1.voting_power_threshold_pct = %v (%T)", bft["voting_power_threshold_pct"], bft["voting_power_threshold_pct"])
	}
}

func TestGenerateDefault(t *testing.T) {
	body, err := DefaultTOML()
	if err != nil {
		t.Fatalf("DefaultTOML: %v", err)
	}
	if len(body) < 100 {
		t.Errorf("body too small (%d bytes)", len(body))
	}
}
