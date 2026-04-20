// internal/sentinel/stats/stats_test.go
package stats_test

import (
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/sentinel/stats"
)

func TestStats_RecordAndSnapshot(t *testing.T) {
	s := stats.New()

	s.Record("rpc", 100, 100) // uncompressed = wire for rpc
	s.Record("rpc", 200, 200)
	s.Record("logs", 500, 120) // compression ratio ~4x

	snap, _ := s.Snapshot()

	if snap["rpc"].LastSnapshotBytes != 300 {
		t.Errorf("rpc LastSnapshotBytes: got %d, want 300", snap["rpc"].LastSnapshotBytes)
	}
	if snap["rpc"].TotalBytes != 300 {
		t.Errorf("rpc TotalBytes: got %d, want 300", snap["rpc"].TotalBytes)
	}
	if snap["rpc"].TotalWireBytes != 300 {
		t.Errorf("rpc TotalWireBytes: got %d, want 300", snap["rpc"].TotalWireBytes)
	}
	if snap["logs"].TotalBytes != 500 {
		t.Errorf("logs TotalBytes (uncompressed): got %d, want 500", snap["logs"].TotalBytes)
	}
	if snap["logs"].TotalWireBytes != 120 {
		t.Errorf("logs TotalWireBytes: got %d, want 120", snap["logs"].TotalWireBytes)
	}
}

func TestStats_SnapshotResetsLastCounters(t *testing.T) {
	s := stats.New()
	s.Record("rpc", 400, 400)

	snap1, _ := s.Snapshot()
	if snap1["rpc"].LastSnapshotBytes != 400 {
		t.Errorf("first snapshot: got %d, want 400", snap1["rpc"].LastSnapshotBytes)
	}

	s.Record("rpc", 100, 100)

	snap2, _ := s.Snapshot()
	if snap2["rpc"].LastSnapshotBytes != 100 {
		t.Errorf("second snapshot LastSnapshotBytes: got %d, want 100", snap2["rpc"].LastSnapshotBytes)
	}
	// Absolute counters keep accumulating across snapshots.
	if snap2["rpc"].TotalBytes != 500 {
		t.Errorf("second snapshot TotalBytes: got %d, want 500", snap2["rpc"].TotalBytes)
	}
}

func TestStats_UptimeIsPositive(t *testing.T) {
	s := stats.New()
	time.Sleep(1 * time.Millisecond)
	_, uptime := s.Snapshot()
	if uptime <= 0 {
		t.Errorf("expected positive uptime, got %v", uptime)
	}
}

func TestStats_EmptySnapshot(t *testing.T) {
	s := stats.New()
	snap, uptime := s.Snapshot()
	if len(snap) != 0 {
		t.Errorf("expected empty snapshot, got %v", snap)
	}
	if uptime < 0 {
		t.Errorf("expected non-negative uptime, got %v", uptime)
	}
}

func TestStats_RecordDropAndRetry(t *testing.T) {
	s := stats.New()
	s.RecordDrop("rpc")
	s.RecordDrop("rpc")
	s.RecordRetry("logs")

	snap, _ := s.Snapshot()

	if snap["rpc"].LastSnapshotDrops != 2 {
		t.Errorf("rpc LastSnapshotDrops: got %d, want 2", snap["rpc"].LastSnapshotDrops)
	}
	if snap["rpc"].TotalDrops != 2 {
		t.Errorf("rpc TotalDrops: got %d, want 2", snap["rpc"].TotalDrops)
	}
	if snap["logs"].LastSnapshotRetries != 1 {
		t.Errorf("logs LastSnapshotRetries: got %d, want 1", snap["logs"].LastSnapshotRetries)
	}
	if snap["logs"].TotalRetries != 1 {
		t.Errorf("logs TotalRetries: got %d, want 1", snap["logs"].TotalRetries)
	}
}

func TestStats_SnapshotResetsLastDropsRetriesButNotTotals(t *testing.T) {
	s := stats.New()
	s.RecordDrop("rpc")
	s.Snapshot() // resets LastSnapshot*
	s.RecordDrop("rpc")
	snap, _ := s.Snapshot()
	if snap["rpc"].LastSnapshotDrops != 1 {
		t.Errorf("LastSnapshotDrops after reset: got %d, want 1", snap["rpc"].LastSnapshotDrops)
	}
	if snap["rpc"].TotalDrops != 2 {
		t.Errorf("TotalDrops across snapshots: got %d, want 2", snap["rpc"].TotalDrops)
	}
}
