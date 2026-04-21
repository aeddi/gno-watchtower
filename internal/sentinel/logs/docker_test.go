package logs

import (
	"strconv"
	"testing"
	"time"
)

func TestNewDockerSource_ReturnsNonNil(t *testing.T) {
	s := NewDockerSource("gnoland", 0)
	if s == nil {
		t.Fatal("expected non-nil DockerSource")
	}
}

func TestBuildLogsOptions_ZeroLookbackFollowsFromNow(t *testing.T) {
	opts := buildLogsOptions(0, time.Unix(1_700_000_000, 0))
	if opts.Tail != "0" {
		t.Errorf("Tail = %q, want %q (follow new only)", opts.Tail, "0")
	}
	if opts.Since != "" {
		t.Errorf("Since = %q, want empty (no lookback)", opts.Since)
	}
}

func TestBuildLogsOptions_LookbackSetsSince(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	opts := buildLogsOptions(60*time.Second, now)
	want := strconv.FormatInt(now.Add(-60*time.Second).Unix(), 10)
	if opts.Since != want {
		t.Errorf("Since = %q, want %q (= now - 60s epoch)", opts.Since, want)
	}
	if opts.Tail != "" {
		t.Errorf("Tail = %q, want empty when Since is set", opts.Tail)
	}
}
