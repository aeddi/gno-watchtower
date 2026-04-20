package stats_test

import (
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/watchtower/stats"
)

func TestStats_RecordAndSnapshot(t *testing.T) {
	st := stats.New()
	st.Record("val-01", "rpc", 100)
	st.Record("val-01", "rpc", 200)
	st.Record("val-01", "logs", 50)
	st.Record("val-02", "metrics", 400)

	snap, uptime := st.Snapshot()

	if uptime <= 0 {
		t.Error("uptime must be positive")
	}
	v01, ok := snap["val-01"]
	if !ok {
		t.Fatal("val-01 missing from snapshot")
	}
	if v01["rpc"].LastHourBytes != 300 {
		t.Errorf("val-01 rpc last_hour_bytes: want 300, got %d", v01["rpc"].LastHourBytes)
	}
	if v01["rpc"].TotalBytes != 300 {
		t.Errorf("val-01 rpc total_bytes: want 300, got %d", v01["rpc"].TotalBytes)
	}
	if v01["logs"].LastHourBytes != 50 {
		t.Errorf("val-01 logs last_hour_bytes: want 50, got %d", v01["logs"].LastHourBytes)
	}
	v02 := snap["val-02"]
	if v02["metrics"].LastHourBytes != 400 {
		t.Errorf("val-02 metrics last_hour_bytes: want 400, got %d", v02["metrics"].LastHourBytes)
	}
}

func TestStats_SnapshotResetsHourlyCounters(t *testing.T) {
	st := stats.New()
	st.Record("val-01", "rpc", 100)

	st.Snapshot() // first snapshot — resets hourly

	st.Record("val-01", "rpc", 50)
	snap, _ := st.Snapshot()

	if snap["val-01"]["rpc"].LastHourBytes != 50 {
		t.Errorf("want 50 after reset, got %d", snap["val-01"]["rpc"].LastHourBytes)
	}
	if snap["val-01"]["rpc"].TotalBytes != 150 {
		t.Errorf("total_bytes must accumulate: want 150, got %d", snap["val-01"]["rpc"].TotalBytes)
	}
}

func TestStats_UptimeGrows(t *testing.T) {
	st := stats.New()
	time.Sleep(10 * time.Millisecond)
	_, uptime := st.Snapshot()
	if uptime < 5*time.Millisecond {
		t.Errorf("uptime too small: %v", uptime)
	}
}
