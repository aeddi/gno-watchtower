package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunBootsAndServesHealth(t *testing.T) {
	vmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	defer vmSrv.Close()
	lokiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"streams","result":[]}}`))
	}))
	defer lokiSrv.Close()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "scribe.toml")
	body := `
[server]
listen_addr = "127.0.0.1:0"
[cluster]
id = "c1"
[storage]
db_path = "` + filepath.Join(dir, "scribe.duckdb") + `"
[sources.victoria_metrics]
url = "` + vmSrv.URL + `"
[sources.loki]
url = "` + lokiSrv.URL + `"
[ingest.fast]
interval = "100ms"
queries = []
[ingest.slow]
interval = "1s"
queries = []
[ingest.logs]
streams = []
overlap_window = "1s"
[writer]
batch_size = 1
batch_window = "50ms"
[anchors]
interval = "1m"
[retention]
hot_window = "30d"
warm_window = "365d"
warm_bucket = "1m"
prune_after = "365d"
compact_at = ""
[backfill]
chunk_size = "1h"
default_lookback = "30d"
resume_stale_after = "5m"
[sse]
slow_subscriber_timeout = "5s"
max_subscribers = 4
[logging]
level = "warn"
format = "json"
`
	if err := os.WriteFile(configPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	addrCh := make(chan string, 1)
	go func() { _ = runCmdImpl(ctx, configPath, addrCh) }()

	select {
	case addr := <-addrCh:
		// Wait briefly for the server to be ready.
		time.Sleep(200 * time.Millisecond)
		resp, err := http.Get("http://" + addr + "/health")
		if err != nil {
			t.Fatalf("health: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("health status = %d", resp.StatusCode)
		}
	case <-ctx.Done():
		t.Fatal("addr never received")
	}
}
