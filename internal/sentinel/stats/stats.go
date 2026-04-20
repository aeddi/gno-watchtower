// internal/sentinel/stats/stats.go
package stats

import (
	"sync"
	"time"
)

// TypeSnapshot holds the stats for one collector type at the time of a snapshot.
// All per-snapshot fields (those named LastSnapshot*) are reset to zero after
// each Snapshot call; TotalBytes is the cumulative count since Stats creation.
type TypeSnapshot struct {
	LastSnapshotBytes   int64
	TotalBytes          int64
	LastSnapshotDrops   int64
	LastSnapshotRetries int64
}

// Stats accumulates bytes-sent counters per collector type for periodic logging.
type Stats struct {
	mu    sync.Mutex
	start time.Time
	data  map[string]*entry
}

type entry struct {
	total      int64
	lastBytes  int64
	drops      int64
	retries    int64
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

// Record adds bytes to the given collector type's counters.
func (s *Stats) Record(collectorType string, bytes int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.getOrCreate(collectorType)
	e.total += int64(bytes)
	e.lastBytes += int64(bytes)
}

// RecordDrop increments the drop counter for the given collector type.
func (s *Stats) RecordDrop(collectorType string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getOrCreate(collectorType).drops++
}

// RecordRetry increments the retry counter for the given collector type.
func (s *Stats) RecordRetry(collectorType string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getOrCreate(collectorType).retries++
}

// Snapshot returns a copy of current stats and resets per-minute counters.
// Also returns the uptime since Stats was created.
func (s *Stats) Snapshot() (map[string]TypeSnapshot, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := make(map[string]TypeSnapshot, len(s.data))
	for k, e := range s.data {
		snap[k] = TypeSnapshot{
			LastSnapshotBytes:   e.lastBytes,
			TotalBytes:          e.total,
			LastSnapshotDrops:   e.drops,
			LastSnapshotRetries: e.retries,
		}
		e.lastBytes = 0
		e.drops = 0
		e.retries = 0
	}
	return snap, time.Since(s.start)
}
