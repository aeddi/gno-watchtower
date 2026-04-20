//go:build !linux || !cgo

// internal/sentinel/logs/journald_stub.go — fallback when journald support is
// not available.
//
// Applies to:
//   - non-Linux builds (darwin, windows) — journald doesn't exist
//   - Linux builds without cgo (the published Docker image, go install without
//     libsystemd-dev) — journald bindings require linking against libsystemd
//
// Users who need journald install the native binary on a Linux host with
// libsystemd-dev installed, or build with CGO_ENABLED=1 from source.
package logs

import (
	"context"
	"fmt"
)

// JournaldSource is a stub that fails at Tail time. NewJournaldSource still
// succeeds so `sentinel doctor` can report "journald: not supported in this
// build" rather than aborting config load.
type JournaldSource struct {
	unit string
}

// NewJournaldSource returns a stub that fails when Tail is called.
func NewJournaldSource(unit string) *JournaldSource {
	return &JournaldSource{unit: unit}
}

// Tail immediately returns an error.
func (s *JournaldSource) Tail(_ context.Context, _ chan<- LogLine) error {
	return fmt.Errorf("journald source is not supported in this build (requires Linux + cgo)")
}
