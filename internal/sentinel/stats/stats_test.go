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

func TestStats_RecordDropByReason(t *testing.T) {
	s := stats.New()
	s.RecordDrop("rpc", "buffer_full")
	s.RecordDrop("rpc", "buffer_full")
	s.RecordDrop("rpc", "retry_exhausted")
	s.RecordDrop("logs", "retry_exhausted")

	snap, _ := s.Snapshot()

	if got := snap["rpc"].LastSnapshotDrops["buffer_full"]; got != 2 {
		t.Errorf("rpc LastSnapshotDrops[buffer_full]: got %d, want 2", got)
	}
	if got := snap["rpc"].LastSnapshotDrops["retry_exhausted"]; got != 1 {
		t.Errorf("rpc LastSnapshotDrops[retry_exhausted]: got %d, want 1", got)
	}
	if got := snap["rpc"].TotalDrops["buffer_full"]; got != 2 {
		t.Errorf("rpc TotalDrops[buffer_full]: got %d, want 2", got)
	}
	if got := snap["logs"].TotalDrops["retry_exhausted"]; got != 1 {
		t.Errorf("logs TotalDrops[retry_exhausted]: got %d, want 1", got)
	}
}

func TestStats_SnapshotResetsLastDropsButNotTotals(t *testing.T) {
	s := stats.New()
	s.RecordDrop("rpc", "buffer_full")
	s.Snapshot() // resets LastSnapshot*
	s.RecordDrop("rpc", "buffer_full")
	snap, _ := s.Snapshot()
	if got := snap["rpc"].LastSnapshotDrops["buffer_full"]; got != 1 {
		t.Errorf("LastSnapshotDrops[buffer_full] after reset: got %d, want 1", got)
	}
	if got := snap["rpc"].TotalDrops["buffer_full"]; got != 2 {
		t.Errorf("TotalDrops[buffer_full] across snapshots: got %d, want 2", got)
	}
}
