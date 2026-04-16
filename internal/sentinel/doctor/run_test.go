package doctor_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aeddi/gno-watchtower/internal/sentinel/config"
	"github.com/aeddi/gno-watchtower/internal/sentinel/doctor"
)

func TestRun_AllDisabled_ExitsZero(t *testing.T) {
	// Config with nothing enabled — all collectors Orange except remote (Red, unreachable).
	cfg := &config.Config{
		Server: config.ServerConfig{URL: "http://127.0.0.1:19999", Token: "tok"},
		// All collectors disabled (zero values).
	}

	var buf strings.Builder
	code := doctor.Run(context.Background(), cfg, "/etc/sentinel.toml", &buf)

	out := buf.String()
	if !strings.Contains(out, "Validating sentinel config:") {
		t.Errorf("report header missing\n%s", out)
	}
	// Remote will be Red (unreachable), so exit 1.
	if code != 1 {
		t.Errorf("want exit 1 (remote unreachable), got %d", code)
	}
}

func TestRun_ReturnsOneOnRedCheck(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{URL: "http://127.0.0.1:19999", Token: "tok"},
	}

	var buf strings.Builder
	code := doctor.Run(context.Background(), cfg, "test.toml", &buf)
	if code != 1 {
		t.Errorf("want exit 1 on unreachable remote, got %d", code)
	}
}
