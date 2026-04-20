// Package termstyle provides ANSI-colored status formatting for terminal output.
// Colors are disabled when NO_COLOR is set or when stdout isn't a TTY so output
// piped into files / CI logs stays clean.
package termstyle

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// ANSI color codes.
const (
	Green  = "\033[32m"
	Red    = "\033[31m"
	Yellow = "\033[33m"
	Dim    = "\033[2m"
	Reset  = "\033[0m"
)

// Status symbols.
const (
	SymOK   = "✔"
	SymFail = "✘"
	SymOff  = "○"
	SymSkip = "~"
)

// colorsEnabled reports whether ANSI color codes should be emitted. Cached on
// first call — env and fd don't change during a process's lifetime.
var colorsEnabled = computeColorsEnabled()

func computeColorsEnabled() bool {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// c wraps s with the given color when colors are enabled; otherwise returns s unchanged.
func c(color, s string) string {
	if !colorsEnabled {
		return s
	}
	return color + s + Reset
}

// OK formats a green success line: "  ✔ label    detail".
func OK(label, detail string) string {
	return fmt.Sprintf("  %s %-20s %s", c(Green, SymOK), label, detail)
}

// Fail formats a red failure line: "  ✘ label    detail".
func Fail(label, detail string) string {
	return fmt.Sprintf("  %s %-20s %s", c(Red, SymFail), label, detail)
}

// Off formats a yellow disabled line: "  ○ label    detail".
func Off(label, detail string) string {
	return fmt.Sprintf("  %s %-20s %s", c(Yellow, SymOff), label, detail)
}

// Skip formats a dim skipped line: "  ~ label    detail".
func Skip(label, detail string) string {
	if !colorsEnabled {
		return fmt.Sprintf("  %s %-20s %s", SymSkip, label, detail)
	}
	return fmt.Sprintf("  %s%s %-20s %s%s", Dim, SymSkip, label, detail, Reset)
}

// SubOK formats an indented green sub-item.
func SubOK(label, detail string) string {
	return fmt.Sprintf("    %s %-18s %s", c(Green, SymOK), label, detail)
}

// SubFail formats an indented red sub-item.
func SubFail(label, detail string) string {
	return fmt.Sprintf("    %s %-18s %s", c(Red, SymFail), label, detail)
}
