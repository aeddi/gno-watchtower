// pkg/logger/logger_test.go
package logger_test

import (
	"log/slog"
	"testing"

	"github.com/gnolang/val-companion/pkg/logger"
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
