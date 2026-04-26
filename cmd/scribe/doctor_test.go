package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorReportsHappyPath(t *testing.T) {
	vmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	defer vmSrv.Close()
	lokiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"streams","result":[]}}`))
	}))
	defer lokiSrv.Close()

	dir := t.TempDir()
	cfg := `
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
interval = "3s"
queries = ["sentinel_validator_online"]
[ingest.slow]
interval = "60s"
queries = []
[ingest.logs]
streams = []
overlap_window = "5s"
[writer]
batch_size = 1
batch_window = "50ms"
[anchors]
interval = "1h"
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
	path := filepath.Join(dir, "scribe.toml")
	if err := os.WriteFile(path, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	var out bytes.Buffer
	if err := doctorCmd([]string{path}, &out); err != nil {
		t.Fatalf("doctorCmd: %v", err)
	}
	for _, want := range []string{"OK", "victoria_metrics", "loki", "store"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("doctor output missing %q\n%s", want, out.String())
		}
	}
}

func TestDoctorReportsBrokenSource(t *testing.T) {
	dir := t.TempDir()
	cfg := `
[server]
listen_addr = "127.0.0.1:0"
[cluster]
id = "c1"
[storage]
db_path = "` + filepath.Join(dir, "scribe.duckdb") + `"
[sources.victoria_metrics]
url = "http://127.0.0.1:1"
[sources.loki]
url = "http://127.0.0.1:1"
[ingest.fast]
interval = "3s"
queries = []
[ingest.slow]
interval = "60s"
queries = []
[ingest.logs]
streams = []
overlap_window = "5s"
[writer]
batch_size = 1
batch_window = "50ms"
[anchors]
interval = "1h"
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
	path := filepath.Join(dir, "scribe.toml")
	_ = os.WriteFile(path, []byte(cfg), 0o644)

	var out bytes.Buffer
	err := doctorCmd([]string{path}, &out)
	if err == nil {
		t.Error("expected doctorCmd to return error when sources unreachable")
	}
	if !strings.Contains(out.String(), "FAIL") {
		t.Errorf("expected FAIL marker in report\n%s", out.String())
	}
}
