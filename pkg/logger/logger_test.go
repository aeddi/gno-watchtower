// pkg/logger/logger_test.go
package logger_test

import (
	"log/slog"
	"testing"

	"github.com/aeddi/gno-watchtower/pkg/logger"
)

func TestNew_ConsoleDoesNotError(t *testing.T) {
	log, err := logger.New(logger.FormatConsole, slog.LevelInfo)
	if err != nil {
		t.Fatalf("New(console): %v", err)
	}
	if log == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestNew_JSONDoesNotError(t *testing.T) {
	log, err := logger.New(logger.FormatJSON, slog.LevelDebug)
	if err != nil {
		t.Fatalf("New(json): %v", err)
	}
	if log == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestNew_UnknownFormatErrors(t *testing.T) {
	_, err := logger.New("unknown", slog.LevelInfo)
	if err == nil {
		t.Fatal("expected error for unknown format")
	}
}

func TestNoop_DiscardsOutput(t *testing.T) {
	log := logger.Noop()
	if log == nil {
		t.Fatal("expected non-nil noop logger")
	}
	// Noop uses io.Discard — verify no panic and output is silent.
	log.Info("this should not appear anywhere")
}

func TestJournalKey_SanitizesForSystemdJournalRegex(t *testing.T) {
	// systemd journal accepts only ^[A-Z_][A-Z0-9_]*$ and silently drops
	// anything else. Leading underscores are reserved for journal internals
	// and also dropped. JournalKey is the helper the journal handler wires
	// into ReplaceAttr so structured slog attrs actually reach the journal.
	cases := []struct {
		name   string
		groups []string
		key    string
		want   string
	}{
		{"simple lowercase", nil, "validator", "VALIDATOR"},
		{"snake case preserved", nil, "last_hour_bytes", "LAST_HOUR_BYTES"},
		{"single group prefix", []string{"rpc"}, "total_bytes", "RPC_TOTAL_BYTES"},
		{"nested groups", []string{"rpc", "latency"}, "p99_ms", "RPC_LATENCY_P99_MS"},
		{"punctuation flattened", nil, "foo-bar.baz", "FOO_BAR_BAZ"},
		{"leading underscore X-prefixed", nil, "_internal", "X_INTERNAL"},
		{"leading digit X_-prefixed", nil, "3rd_try", "X_3RD_TRY"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := logger.JournalKey(tc.groups, tc.key)
			if got != tc.want {
				t.Errorf("JournalKey(%v, %q) = %q, want %q", tc.groups, tc.key, got, tc.want)
			}
		})
	}
}

func TestLevelRank(t *testing.T) {
	tests := []struct {
		level string
		want  int
	}{
		{"debug", 0},
		{"info", 1},
		{"warn", 2},
		{"error", 3},
		{"unknown", 1},
	}
	for _, tt := range tests {
		if got := logger.LevelRank(tt.level); got != tt.want {
			t.Errorf("LevelRank(%q) = %d, want %d", tt.level, got, tt.want)
		}
	}
}
