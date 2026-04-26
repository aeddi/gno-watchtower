// Package cache provides an in-memory live state cache mapping (cluster, subject)
// to projected State. Updated by the normalizer in tandem with persisted writes;
// rebuilt on startup from latest anchor + event replay.
package cache

import (
	"sort"
	"sync"

	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

// State is the projected structured state for one (cluster, subject).
// Fast scalars come from samples, not from the cache.
type State struct {
	Peers          map[string]types.Provenance // peer_id -> provenance of last connect event
	ValsetView     []map[string]any            // [{address, voting_power}, ...]
	ConsensusLocks map[string]any              // {locked_block, valid_block, ...}
	ConfigHash     string
	Extras         map[string]any
}

type key struct{ cluster, subject string }

// Cache is the in-memory live state cache.
type Cache struct {
	mu      sync.RWMutex
	states  map[key]State
	through map[key]string // last event_id projected
}

// New returns an empty Cache.
func New() *Cache {
	return &Cache{
		states:  map[key]State{},
		through: map[key]string{},
	}
}

// Put stores s for (cluster, subject) and records eventsThrough as the last
// event_id applied. A blank eventsThrough is ignored.
func (c *Cache) Put(cluster, subject string, s State, eventsThrough string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.states[key{cluster, subject}] = s
	if eventsThrough != "" {
		c.through[key{cluster, subject}] = eventsThrough
	}
}

// Get returns the cached State for (cluster, subject) and whether it existed.
func (c *Cache) Get(cluster, subject string) (State, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s, ok := c.states[key{cluster, subject}]
	return s, ok
}

// EventsThrough returns the last event_id applied to the cache for this subject.
func (c *Cache) EventsThrough(cluster, subject string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.through[key{cluster, subject}]
}

// Subjects returns all known subjects for the given cluster, sorted.
func (c *Cache) Subjects(cluster string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, 0)
	for k := range c.states {
		if k.cluster == cluster {
			out = append(out, k.subject)
		}
	}
	sort.Strings(out)
	return out
}
