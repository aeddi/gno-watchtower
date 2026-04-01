// pkg/logger/logger.go
package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"

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
		h, err := slogjournal.NewHandler(&slogjournal.Options{Level: level})
		if err != nil {
			return nil, fmt.Errorf("journal handler: %w", err)
		}
		return slog.New(h), nil
	default:
		return nil, fmt.Errorf("unknown log format %q: must be console, json, or journal", format)
	}
}

// Noop returns a logger that discards all output. Use in tests.
func Noop() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
