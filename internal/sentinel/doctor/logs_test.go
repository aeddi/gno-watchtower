package doctor_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/sentinel/config"
	"github.com/aeddi/gno-watchtower/internal/sentinel/doctor"
	"github.com/aeddi/gno-watchtower/internal/sentinel/logs"
)

// staticSource is a test Source that emits a fixed set of lines then blocks.
type staticSource struct {
	lines []logs.LogLine
}

func (s *staticSource) Tail(ctx context.Context, out chan<- logs.LogLine) error {
	for _, l := range s.lines {
		select {
		case out <- l:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	<-ctx.Done()
	return ctx.Err()
}

func jsonLine(level, msg string) logs.LogLine {
	raw, _ := json.Marshal(map[string]string{"level": level, "msg": msg})
	return logs.LogLine{Level: level, Raw: raw}
}

func TestCheckLogs_GreenWhenLinesAtMinLevel(t *testing.T) {
	src := &staticSource{lines: []logs.LogLine{
		jsonLine("info", "hello"),
		jsonLine("warn", "careful"),
	}}
	cfg := config.LogsConfig{MinLevel: "info"}

	r := doctor.CheckLogs(context.Background(), src, cfg, "info")
	if r.Status != doctor.StatusGreen {
		t.Errorf("want GREEN, got %s: %s", r.Status, r.Detail)
	}
}

func TestCheckLogs_RedWhenNoLinesAtMinLevel(t *testing.T) {
	src := &staticSource{lines: []logs.LogLine{
		jsonLine("debug", "verbose"),
	}}
	cfg := config.LogsConfig{MinLevel: "warn"}

	r := doctor.CheckLogs(context.Background(), src, cfg, "warn")
	if r.Status != doctor.StatusRed {
		t.Errorf("want RED (no lines at warn+), got %s: %s", r.Status, r.Detail)
	}
}

func TestCheckLogs_RedWhenNoLines(t *testing.T) {
	src := &staticSource{lines: nil} // emits nothing
	cfg := config.LogsConfig{MinLevel: "info"}

	r := doctor.CheckLogs(context.Background(), src, cfg, "info")
	if r.Status != doctor.StatusRed {
		t.Errorf("want RED (no lines), got %s: %s", r.Status, r.Detail)
	}
}

func TestCheckLogs_RedWhenInvalidJSON(t *testing.T) {
	src := &staticSource{lines: []logs.LogLine{
		{Level: "info", Raw: []byte("not json at all")},
	}}
	cfg := config.LogsConfig{MinLevel: "info"}

	r := doctor.CheckLogs(context.Background(), src, cfg, "info")
	if r.Status != doctor.StatusRed {
		t.Errorf("want RED (invalid JSON), got %s: %s", r.Status, r.Detail)
	}
}

func TestCheckLogs_RespectsTimeout(t *testing.T) {
	// Source that never sends anything — check must complete via timeout, not hang.
	src := &staticSource{lines: nil}
	cfg := config.LogsConfig{MinLevel: "info"}

	start := time.Now()
	r := doctor.CheckLogs(context.Background(), src, cfg, "info")
	elapsed := time.Since(start)

	if elapsed > 5*time.Second {
		t.Errorf("check took too long: %v", elapsed)
	}
	if r.Status != doctor.StatusRed {
		t.Errorf("want RED (no lines), got %s", r.Status)
	}
}
