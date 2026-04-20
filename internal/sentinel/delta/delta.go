// internal/sentinel/delta/delta.go
package delta

import (
	"crypto/sha256"
	"sync"
)

// Delta tracks the hash of the last seen value per key.
// Changed returns true (and updates the stored hash) if data differs from the last call for that key.
type Delta struct {
	mu     sync.Mutex
	hashes map[string][sha256.Size]byte
}

func NewDelta() *Delta {
	return &Delta{hashes: make(map[string][sha256.Size]byte)}
}

func (d *Delta) Changed(key string, data []byte) bool {
	h := sha256.Sum256(data)
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.hashes[key] == h {
		return false
	}
	d.hashes[key] = h
	return true
}
