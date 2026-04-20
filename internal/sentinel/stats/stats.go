// internal/sentinel/stats/stats.go
package stats

import (
	"sync"
	"time"
)

// TypeSnapshot holds the stats for one collector type at the time of a snapshot.
//
// Absolute counters (Total*) are monotonic since Stats creation and never reset.
// They're suitable for Prometheus counter semantics (the backend takes rate()
// or increase() to derive deltas). Use these when exporting as metrics.
//
// Per-snapshot counters (LastSnapshot*) reset to zero after each Snapshot call.
// They're suitable for periodic human-readable log lines ("this minute we sent
// X bytes"). Use these for sentinel stdout.
//
// Wire vs uncompressed: for types that don't compress (rpc, metrics, otlp),
// wire == uncompressed. For logs, wire is the zstd-compressed payload size.
type TypeSnapshot struct {
	TotalBytes          int64
	TotalWireBytes      int64
	TotalDrops          int64
	TotalRetries        int64
	LastSnapshotBytes   int64
	LastSnapshotDrops   int64
	LastSnapshotRetries int64
}

// Stats accumulates bytes-sent counters per collector type for periodic logging
// and Prometheus export.
type Stats struct {
	mu    sync.Mutex
	start time.Time
	data  map[string]*entry
}

type entry struct {
	totalBytes     int64
	totalWireBytes int64
	totalDrops     int64
	totalRetries   int64
	lastBytes      int64
	lastDrops      int64
	lastRetries    int64
}

// New creates a new Stats accumulator. The start time is set to now.
func New() *Stats {
	return &Stats{
		start: time.Now(),
		data:  make(map[string]*entry),
	}
}

func (s *Stats) getOrCreate(collectorType string) *entry {
	e, ok := s.data[collectorType]
	if !ok {
		e = &entry{}
		s.data[collectorType] = e
	}
	return e
}

// Record adds a sent payload to the counters. uncompressed is the payload size
// before the wire encoding (JSON-marshaled, pre-zstd for logs); wire is what
// actually went on the network (equal to uncompressed for non-compressed types).
func (s *Stats) Record(collectorType string, uncompressed, wire int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.getOrCreate(collectorType)
	e.totalBytes += int64(uncompressed)
	e.totalWireBytes += int64(wire)
	e.lastBytes += int64(uncompressed)
}

// RecordDrop increments the drop counter for the given collector type.
func (s *Stats) RecordDrop(collectorType string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.getOrCreate(collectorType)
	e.totalDrops++
	e.lastDrops++
}

// RecordRetry increments the retry counter for the given collector type.
func (s *Stats) RecordRetry(collectorType string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.getOrCreate(collectorType)
	e.totalRetries++
	e.lastRetries++
}

// Snapshot returns a copy of current stats and resets per-snapshot counters.
// Absolute counters (Total*) are returned unchanged and keep accumulating.
// Also returns the uptime since Stats was created.
func (s *Stats) Snapshot() (map[string]TypeSnapshot, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := make(map[string]TypeSnapshot, len(s.data))
	for k, e := range s.data {
		snap[k] = TypeSnapshot{
			TotalBytes:          e.totalBytes,
			TotalWireBytes:      e.totalWireBytes,
			TotalDrops:          e.totalDrops,
			TotalRetries:        e.totalRetries,
			LastSnapshotBytes:   e.lastBytes,
			LastSnapshotDrops:   e.lastDrops,
			LastSnapshotRetries: e.lastRetries,
		}
		e.lastBytes = 0
		e.lastDrops = 0
		e.lastRetries = 0
	}
	return snap, time.Since(s.start)
}
