// Package levels provides log level rank comparisons shared across packages.
package levels

// Rank returns a numeric rank for log level filtering.
// Unknown levels → 1 (info).
func Rank(level string) int {
	switch level {
	case "debug":
		return 0
	case "info":
		return 1
	case "warn":
		return 2
	case "error":
		return 3
	default:
		return 1
	}
}
