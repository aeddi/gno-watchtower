package rpc

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

// Delta tracks the hash of the last seen value per key.
// Changed returns true (and updates the stored hash) if data differs from the last call for that key.
type Delta struct {
	mu     sync.Mutex
	hashes map[string]string
}

func NewDelta() *Delta {
	return &Delta{hashes: make(map[string]string)}
}

func (d *Delta) Changed(key string, data []byte) bool {
	h := hashBytes(data)
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.hashes[key] == h {
		return false
	}
	d.hashes[key] = h
	return true
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
