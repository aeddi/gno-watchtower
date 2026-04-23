// pkg/logger/logger.go
package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	slogjournal "github.com/systemd/slog-journal"
)

// Format selects the log output format.
type Format string

const (
	FormatConsole Format = "console" // slog default text handler (human-readable)
	FormatJSON    Format = "json"    // slog JSON handler
	FormatJournal Format = "journal" // systemd journal via slog-journal
)

// New creates a *slog.Logger writing to stderr with the given format and minimum level.
// Returns an error for unknown formats or if the journal handler fails to initialise.
func New(format Format, level slog.Level) (*slog.Logger, error) {
	opts := &slog.HandlerOptions{Level: level}
	switch format {
	case FormatConsole:
		return slog.New(slog.NewTextHandler(os.Stderr, opts)), nil
	case FormatJSON:
		return slog.New(slog.NewJSONHandler(os.Stderr, opts)), nil
	case FormatJournal:
		// Without ReplaceAttr the journal silently drops every structured attr
		// we emit because our keys are lowercase (validator, total_bytes, …)
		// and the journal only accepts ^[A-Z_][A-Z0-9_]*$.
		h, err := slogjournal.NewHandler(&slogjournal.Options{
			Level: level,
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				a.Key = JournalKey(groups, a.Key)
				return a
			},
		})
		if err != nil {
			return nil, fmt.Errorf("journal handler: %w", err)
		}
		return slog.New(h), nil
	default:
		return nil, fmt.Errorf("unknown log format %q: must be console, json, or journal", format)
	}
}

// JournalKey rewrites an slog attr key (with its group path) into a form the
// systemd journal accepts: ^[A-Z_][A-Z0-9_]*$. Non-alphanumeric characters
// collapse to '_'; lowercase letters uppercase; a leading underscore gets an
// 'X' prefix (journal reserves leading underscores for its own fields); a
// leading digit gets an 'X_' prefix. Nested groups are joined with '_' so
// slog.Group("rpc", "latency_ms", ...) surfaces as RPC_LATENCY_MS.
func JournalKey(groups []string, key string) string {
	var b strings.Builder
	for _, g := range groups {
		writeSanitized(&b, g)
		b.WriteByte('_')
	}
	writeSanitized(&b, key)
	out := b.String()
	if strings.HasPrefix(out, "_") {
		return "X" + out
	}
	if len(out) > 0 && out[0] >= '0' && out[0] <= '9' {
		return "X_" + out
	}
	return out
}

func writeSanitized(b *strings.Builder, s string) {
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		case r >= 'a' && r <= 'z':
			b.WriteRune(r - 'a' + 'A')
		default:
			b.WriteByte('_')
		}
	}
}

// Noop returns a logger that discards all output. Use in tests.
func Noop() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// LevelRank returns a numeric rank for log level comparisons.
// Unknown levels return 1 (info rank).
func LevelRank(level string) int {
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

// ParseLevel converts a string level name to a slog.Level.
func ParseLevel(s string) (slog.Level, error) {
	switch s {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unknown level %q: must be debug, info, warn, or error", s)
	}
}
