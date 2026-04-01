//go:build !linux

// internal/sentinel/logs/journald_other.go (temporary — will be expanded in Task 5)
package logs

import (
	"context"
	"fmt"
)

type JournaldSource struct{ unit string }

func NewJournaldSource(unit string) *JournaldSource { return &JournaldSource{unit: unit} }
func (s *JournaldSource) Tail(_ context.Context, _ chan<- LogLine) error {
	return fmt.Errorf("journald source is not supported on this platform")
}
