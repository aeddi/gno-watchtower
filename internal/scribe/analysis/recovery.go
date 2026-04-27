package analysis

import "sync"

// Tracker is per-rule, in-memory state that pairs open and recovered
// emissions for sustained-condition rules. Rules embed it directly:
//
//	type BFTAtRiskRule struct{ tracker analysis.Tracker }
//
// On startup the engine seeds each rule's tracker from currently-open
// diagnostics in DuckDB via Rehydrate; see analysis.Engine.Rehydrate.
type Tracker struct {
	mu   sync.Mutex
	open map[string]string // key -> opening event_id
}

// Open marks key as currently in-incident. Returns true if this is a NEW
// incident (rule should emit state=open with eventID), false if key was
// already open (caller should NOT emit a duplicate).
func (t *Tracker) Open(key, eventID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.open == nil {
		t.open = map[string]string{}
	}
	if _, exists := t.open[key]; exists {
		return false
	}
	t.open[key] = eventID
	return true
}

// Recovered marks key as no longer in-incident. Returns the opening
// event_id (caller emits state=recovered with Recovers=<returned id>), or ""
// if there was no open incident for this key.
func (t *Tracker) Recovered(key string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	openID, ok := t.open[key]
	if !ok {
		return ""
	}
	delete(t.open, key)
	return openID
}

// Rehydrate seeds the tracker from currently-open incidents discovered at
// startup. Replaces existing state.
func (t *Tracker) Rehydrate(entries map[string]string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.open = make(map[string]string, len(entries))
	for k, v := range entries {
		t.open[k] = v
	}
}

// OpenCount returns the number of currently-open incidents. Used to roll up
// the scribe_analysis_open_incidents{code} gauge.
func (t *Tracker) OpenCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.open)
}

// Snapshot returns a copy of the current open-incident map. The caller may
// mutate the returned map without affecting tracker state.
func (t *Tracker) Snapshot() map[string]string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make(map[string]string, len(t.open))
	for k, v := range t.open {
		out[k] = v
	}
	return out
}
