package analysis

import "testing"

func TestTrackerOpenIsIdempotentForSameKey(t *testing.T) {
	var tr Tracker
	if !tr.Open("c1|val-1", "01J0") {
		t.Fatal("first Open should report new")
	}
	if tr.Open("c1|val-1", "01J1") {
		t.Errorf("second Open for same key should report existing, not new")
	}
	if got := tr.OpenCount(); got != 1 {
		t.Errorf("OpenCount = %d, want 1", got)
	}
}

func TestTrackerRecoveredReturnsOpeningEventID(t *testing.T) {
	var tr Tracker
	tr.Open("c1", "01J0")
	got := tr.Recovered("c1")
	if got != "01J0" {
		t.Errorf("Recovered returned %q, want 01J0", got)
	}
	if tr.OpenCount() != 0 {
		t.Errorf("OpenCount after Recovered = %d, want 0", tr.OpenCount())
	}
}

func TestTrackerRecoveredOnUnknownKeyReturnsEmpty(t *testing.T) {
	var tr Tracker
	if got := tr.Recovered("unknown"); got != "" {
		t.Errorf("Recovered(unknown) = %q, want empty", got)
	}
}

func TestTrackerRehydrateSeedsState(t *testing.T) {
	var tr Tracker
	tr.Rehydrate(map[string]string{"c1|val-1": "01J0", "c1": "01JC"})
	if tr.Open("c1|val-1", "01J9") {
		t.Errorf("Open after Rehydrate should report existing, not new")
	}
	if got := tr.Recovered("c1"); got != "01JC" {
		t.Errorf("Recovered after Rehydrate = %q, want 01JC", got)
	}
}

func TestTrackerSnapshotReturnsCopy(t *testing.T) {
	var tr Tracker
	tr.Open("c1|val-1", "01J0")
	snap := tr.Snapshot()
	snap["c1|val-1"] = "MUTATED"
	if tr.Open("c1|val-1", "01J9") {
		t.Errorf("internal state was modified through Snapshot return value")
	}
}
