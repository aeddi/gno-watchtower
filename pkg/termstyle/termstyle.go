// Package termstyle provides ANSI-colored status formatting for terminal output.
package termstyle

import "fmt"

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

// OK formats a green success line: "  ✔ label    detail".
func OK(label, detail string) string {
	return fmt.Sprintf("  %s%s%s %-20s %s", Green, SymOK, Reset, label, detail)
}

// Fail formats a red failure line: "  ✘ label    detail".
func Fail(label, detail string) string {
	return fmt.Sprintf("  %s%s%s %-20s %s", Red, SymFail, Reset, label, detail)
}

// Off formats a yellow disabled line: "  ○ label    detail".
func Off(label, detail string) string {
	return fmt.Sprintf("  %s%s%s %-20s %s", Yellow, SymOff, Reset, label, detail)
}

// Skip formats a dim skipped line: "  ~ label    detail".
func Skip(label, detail string) string {
	return fmt.Sprintf("  %s%s %-20s %s%s", Dim, SymSkip, label, detail, Reset)
}

// SubOK formats an indented green sub-item.
func SubOK(label, detail string) string {
	return fmt.Sprintf("    %s%s%s %-18s %s", Green, SymOK, Reset, label, detail)
}

// SubFail formats an indented red sub-item.
func SubFail(label, detail string) string {
	return fmt.Sprintf("    %s%s%s %-18s %s", Red, SymFail, Reset, label, detail)
}
