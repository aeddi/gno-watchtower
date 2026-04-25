// Package eventid produces deterministic, content-addressed ULIDs for events.
//
// The 48-bit time prefix means later timestamps sort larger; the 80-bit entropy
// suffix is the SHA-256 prefix of (kind | subject | payload), so the same input
// always yields the same ULID. Combined with INSERT OR IGNORE on event_id, this
// makes re-ingesting the same event a no-op (idempotent backfill, restart, retry).
package eventid

import (
	"crypto/sha256"
	"time"

	"github.com/oklog/ulid/v2"
)

// Derive returns a deterministic ULID built from (time, hash(kind | subject | payload)).
func Derive(at time.Time, kind, subject string, payload []byte) string {
	h := sha256.New()
	h.Write([]byte(kind))
	h.Write([]byte{0})
	h.Write([]byte(subject))
	h.Write([]byte{0})
	h.Write(payload)
	sum := h.Sum(nil)

	var entropy [10]byte
	copy(entropy[:], sum[:10])

	id := ulid.ULID{}
	// SetTime / SetEntropy only error on length / overflow which we control;
	// ignore the error returns deliberately.
	_ = id.SetTime(uint64(at.UnixMilli()))
	_ = id.SetEntropy(entropy[:])
	return id.String()
}
