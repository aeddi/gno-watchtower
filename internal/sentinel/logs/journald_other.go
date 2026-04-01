//go:build !linux

// internal/sentinel/logs/journald_other.go
package logs

import (
	"context"
	"fmt"
)

// JournaldSource is not supported on non-Linux platforms.
type JournaldSource struct {
	unit string
}

// NewJournaldSource returns a stub that fails on non-Linux platforms.
func NewJournaldSource(unit string) *JournaldSource {
	return &JournaldSource{unit: unit}
}

// Tail immediately returns an error on non-Linux platforms.
func (s *JournaldSource) Tail(_ context.Context, _ chan<- LogLine) error {
	return fmt.Errorf("journald source is not supported on this platform (Linux only)")
}
