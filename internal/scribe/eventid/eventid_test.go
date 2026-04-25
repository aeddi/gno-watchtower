package eventid

import (
	"testing"
	"time"
)

func TestDeriveDeterministic(t *testing.T) {
	at := time.Date(2026, 4, 25, 10, 14, 33, 214_000_000, time.UTC)
	payload := []byte(`{"height":1234,"round":0}`)
	a := Derive(at, "validator.vote_missed", "node-1", payload)
	b := Derive(at, "validator.vote_missed", "node-1", payload)
	if a != b {
		t.Errorf("Derive should be deterministic: %q vs %q", a, b)
	}
	if len(a) != 26 {
		t.Errorf("ULID length = %d, want 26", len(a))
	}
}

func TestDeriveDifferentInputsDifferentIDs(t *testing.T) {
	at := time.Date(2026, 4, 25, 10, 14, 33, 0, time.UTC)
	a := Derive(at, "validator.vote_missed", "node-1", []byte(`{"h":1}`))
	b := Derive(at, "validator.vote_missed", "node-1", []byte(`{"h":2}`))
	if a == b {
		t.Error("different payloads must yield different IDs")
	}
}

func TestDeriveTimeSortable(t *testing.T) {
	t1 := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Second)
	a := Derive(t1, "x", "y", []byte("z"))
	b := Derive(t2, "x", "y", []byte("z"))
	if a >= b {
		t.Errorf("later timestamp must yield lexicographically larger ULID: %q vs %q", a, b)
	}
}
