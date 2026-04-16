// internal/sentinel/stats/stats_test.go
package stats_test

import (
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/sentinel/stats"
)

func TestStats_RecordAndSnapshot(t *testing.T) {
	s := stats.New()

	s.Record("rpc", 100)
	s.Record("rpc", 200)
	s.Record("logs", 500)

	snap, _ := s.Snapshot()

	if snap["rpc"].LastMinuteBytes != 300 {
		t.Errorf("rpc LastMinuteBytes: got %d, want 300", snap["rpc"].LastMinuteBytes)
	}
	if snap["rpc"].TotalBytes != 300 {
		t.Errorf("rpc TotalBytes: got %d, want 300", snap["rpc"].TotalBytes)
	}
	if snap["logs"].LastMinuteBytes != 500 {
		t.Errorf("logs LastMinuteBytes: got %d, want 500", snap["logs"].LastMinuteBytes)
	}
}

func TestStats_SnapshotResetsMinuteCounter(t *testing.T) {
	s := stats.New()
	s.Record("rpc", 400)

	snap1, _ := s.Snapshot()
	if snap1["rpc"].LastMinuteBytes != 400 {
		t.Errorf("first snapshot: got %d, want 400", snap1["rpc"].LastMinuteBytes)
	}

	// Record more after first snapshot.
	s.Record("rpc", 100)

	snap2, _ := s.Snapshot()
	// Last-minute should only reflect post-snapshot traffic.
	if snap2["rpc"].LastMinuteBytes != 100 {
		t.Errorf("second snapshot LastMinuteBytes: got %d, want 100", snap2["rpc"].LastMinuteBytes)
	}
	// Total accumulates across snapshots.
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

	if snap["rpc"].Drops != 2 {
		t.Errorf("rpc Drops: got %d, want 2", snap["rpc"].Drops)
	}
	if snap["logs"].Retries != 1 {
		t.Errorf("logs Retries: got %d, want 1", snap["logs"].Retries)
	}
}

func TestStats_DropRetryResetOnSnapshot(t *testing.T) {
	s := stats.New()
	s.RecordDrop("rpc")
	s.Snapshot() // resets
	s.RecordDrop("rpc")
	snap, _ := s.Snapshot()
	if snap["rpc"].Drops != 1 {
		t.Errorf("expected 1 drop after reset, got %d", snap["rpc"].Drops)
	}
}
