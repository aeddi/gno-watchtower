package server

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
)

func TestRejectLogLevel(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want slog.Level
	}{
		{"io_eof", io.EOF, slog.LevelDebug},
		{"io_unexpected_eof", io.ErrUnexpectedEOF, slog.LevelDebug},
		{"wrapped_eof", fmt.Errorf("noise handshake recv msg 1: %w", io.EOF), slog.LevelDebug},
		{"other_error", errors.New("bad noise key"), slog.LevelWarn},
		{"nil", nil, slog.LevelWarn},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rejectLogLevel(tc.err)
			if got != tc.want {
				t.Fatalf("rejectLogLevel(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
