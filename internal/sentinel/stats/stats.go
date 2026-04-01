// internal/sentinel/stats/stats.go
package stats

import (
	"sync"
	"time"
)

// TypeSnapshot holds the stats for one collector type at the time of a snapshot.
type TypeSnapshot struct {
	LastMinuteBytes int64
	TotalBytes      int64
}

// Stats accumulates bytes-sent counters per collector type for periodic logging.
type Stats struct {
	mu    sync.Mutex
	start time.Time
	data  map[string]*entry
}

type entry struct {
	total  int64
	minute int64
}

// New creates a new Stats accumulator. The start time is set to now.
func New() *Stats {
	return &Stats{
		start: time.Now(),
		data:  make(map[string]*entry),
	}
}

// Record adds bytes to the given collector type's counters.
func (s *Stats) Record(collectorType string, bytes int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.data[collectorType]
	if !ok {
		e = &entry{}
		s.data[collectorType] = e
	}
	e.total += int64(bytes)
	e.minute += int64(bytes)
}

// Snapshot returns a copy of current stats and resets per-minute counters.
// Also returns the uptime since Stats was created.
func (s *Stats) Snapshot() (map[string]TypeSnapshot, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := make(map[string]TypeSnapshot, len(s.data))
	for k, e := range s.data {
		snap[k] = TypeSnapshot{
			LastMinuteBytes: e.minute,
			TotalBytes:      e.total,
		}
		e.minute = 0
	}
	return snap, time.Since(s.start)
}
