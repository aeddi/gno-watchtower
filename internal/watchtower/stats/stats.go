// Package stats tracks per-validator, per-type byte counts with hourly snapshots.
package stats

import (
	"sync"
	"time"
)

// TypeSnapshot holds byte counts for one validator+type combination.
type TypeSnapshot struct {
	LastHourBytes int
	TotalBytes    int
}

type entry struct {
	lastHour int
	total    int
}

// Stats tracks per-validator, per-type byte counts.
type Stats struct {
	mu      sync.Mutex
	data    map[string]map[string]*entry // validator → type → entry
	startAt time.Time
}

// New creates a Stats instance.
func New() *Stats {
	return &Stats{
		data:    make(map[string]map[string]*entry),
		startAt: time.Now(),
	}
}

// Record adds bytes for the given validator and collector type.
func (s *Stats) Record(validator, collectorType string, bytes int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data[validator] == nil {
		s.data[validator] = make(map[string]*entry)
	}
	e := s.data[validator][collectorType]
	if e == nil {
		e = &entry{}
		s.data[validator][collectorType] = e
	}
	e.lastHour += bytes
	e.total += bytes
}

// Snapshot returns a copy of per-validator stats and the uptime.
// Resets the per-hour counters after copying.
func (s *Stats) Snapshot() (map[string]map[string]TypeSnapshot, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]map[string]TypeSnapshot, len(s.data))
	for validator, types := range s.data {
		out[validator] = make(map[string]TypeSnapshot, len(types))
		for typ, e := range types {
			out[validator][typ] = TypeSnapshot{LastHourBytes: e.lastHour, TotalBytes: e.total}
			e.lastHour = 0
		}
	}
	return out, time.Since(s.startAt)
}
