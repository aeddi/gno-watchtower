// internal/sentinel/doctor/report.go
package doctor

import (
	"fmt"

	"github.com/aeddi/gno-watchtower/pkg/termstyle"
)

// Format controls how Run renders check results.
type Format int

const (
	// FormatStyled is the default TTY-friendly output with Unicode symbols
	// and ANSI colors (colors auto-disable when stdout isn't a TTY).
	FormatStyled Format = iota
	// FormatPlain emits ASCII-bracketed status tags ([GREEN]/[RED]/...)
	// and never uses Unicode or ANSI. Intended for log pipelines, CI parsers,
	// and `grep -c "\[RED\]"`-style operator tooling.
	FormatPlain
)

func formatResult(r CheckResult, f Format) string {
	if f == FormatPlain {
		return fmt.Sprintf("  [%s] %-20s %s", r.Status, r.Name, r.Detail)
	}
	switch r.Status {
	case StatusGreen:
		return termstyle.OK(r.Name, r.Detail)
	case StatusRed:
		return termstyle.Fail(r.Name, r.Detail)
	case StatusOrange:
		return termstyle.Off(r.Name, r.Detail)
	case StatusGrey:
		return termstyle.Skip(r.Name, r.Detail)
	default:
		return termstyle.Fail(r.Name, r.Detail)
	}
}
