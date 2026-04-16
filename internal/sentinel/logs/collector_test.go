package logs_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/sentinel/logs"
	"github.com/aeddi/gno-watchtower/pkg/logger"
	"github.com/aeddi/gno-watchtower/pkg/protocol"
)

// fakeSource emits the given lines then blocks until ctx is cancelled.
type fakeSource struct {
	lines []logs.LogLine
}

func (f *fakeSource) Tail(ctx context.Context, out chan<- logs.LogLine) error {
	for _, line := range f.lines {
		select {
		case out <- line:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	<-ctx.Done()
	return ctx.Err()
}

func makeLine(level, msg string) logs.LogLine {
	raw, _ := json.Marshal(map[string]string{"level": level, "msg": msg})
	return logs.LogLine{Level: level, Raw: raw}
}

// drainPayloads reads from ch until no new items arrive within waitFor.
func drainPayloads(ch <-chan protocol.LogPayload, waitFor time.Duration) []protocol.LogPayload {
	var result []protocol.LogPayload
	for {
		select {
		case p := <-ch:
			result = append(result, p)
		case <-time.After(waitFor):
			return result
		}
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`{"level":"debug","msg":"x"}`, "debug"},
		{`{"level":"info","msg":"x"}`, "info"},
		{`{"level":"warn","msg":"x"}`, "warn"},
		{`{"level":"error","msg":"x"}`, "error"},
		{`{"level":"unknown","msg":"x"}`, "info"}, // unknown → info
		{`{"msg":"no level"}`, "info"},             // missing → info
		{`not json`, "info"},                       // invalid JSON → info
	}
	for _, tt := range tests {
		got := logs.ParseLevel(json.RawMessage(tt.input))
		if got != tt.want {
			t.Errorf("ParseLevel(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCollector_FiltersBelowMinLevel(t *testing.T) {
	src := &fakeSource{lines: []logs.LogLine{
		makeLine("debug", "debug msg"),
		makeLine("info", "info msg"),
		makeLine("warn", "warn msg"),
		makeLine("error", "error msg"),
	}}
	out := make(chan protocol.LogPayload, 10)
	c := logs.NewCollector(src, "warn", 1024*1024, 50*time.Millisecond, out, logger.Noop())

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	go c.Run(ctx)

	payloads := drainPayloads(out, 150*time.Millisecond)

	counts := make(map[string]int)
	for _, p := range payloads {
		counts[p.Level] += len(p.Lines)
	}
	if counts["debug"] > 0 || counts["info"] > 0 {
		t.Errorf("expected debug and info filtered out, got counts: %v", counts)
	}
	if counts["warn"] == 0 {
		t.Error("expected warn lines in output")
	}
	if counts["error"] == 0 {
		t.Error("expected error lines in output")
	}
}

func TestCollector_BatchesByTimeout(t *testing.T) {
	src := &fakeSource{lines: []logs.LogLine{makeLine("info", "only line")}}
	out := make(chan protocol.LogPayload, 10)
	// Large batch size so timeout triggers first.
	c := logs.NewCollector(src, "info", 1024*1024, 30*time.Millisecond, out, logger.Noop())

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	go c.Run(ctx)

	select {
	case p := <-out:
		if p.Level != "info" {
			t.Errorf("Level: got %q, want %q", p.Level, "info")
		}
		if len(p.Lines) != 1 {
			t.Errorf("Lines: got %d, want 1", len(p.Lines))
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected payload within 200ms (batch timeout not triggered)")
	}
}

func TestCollector_BatchesBySize(t *testing.T) {
	// batchSize = 1 byte: any line exceeds it and triggers an immediate flush.
	src := &fakeSource{lines: []logs.LogLine{
		makeLine("info", "line 1"),
		makeLine("info", "line 2"),
		makeLine("info", "line 3"),
	}}
	out := make(chan protocol.LogPayload, 10)
	c := logs.NewCollector(src, "info", 1, time.Second, out, logger.Noop())

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go c.Run(ctx)

	// Should receive batches before the 1s timeout.
	select {
	case p := <-out:
		if len(p.Lines) == 0 {
			t.Error("expected at least one line in size-triggered batch")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected size-triggered batch, not timeout-triggered")
	}
}

func TestCollector_GroupsByLevel(t *testing.T) {
	src := &fakeSource{lines: []logs.LogLine{
		makeLine("info", "info 1"),
		makeLine("warn", "warn 1"),
		makeLine("info", "info 2"),
	}}
	out := make(chan protocol.LogPayload, 10)
	c := logs.NewCollector(src, "info", 1024*1024, 50*time.Millisecond, out, logger.Noop())

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	go c.Run(ctx)

	payloads := drainPayloads(out, 150*time.Millisecond)
	counts := make(map[string]int)
	for _, p := range payloads {
		counts[p.Level] += len(p.Lines)
	}
	if counts["info"] != 2 {
		t.Errorf("info count: got %d, want 2", counts["info"])
	}
	if counts["warn"] != 1 {
		t.Errorf("warn count: got %d, want 1", counts["warn"])
	}
}
