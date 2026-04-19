package logs_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/sentinel/logs"
)

func TestEnsureJSON_PassesThroughValidJSON(t *testing.T) {
	raw := []byte(`{"level":"info","msg":"hello","ts":1234567890.5}`)
	got := logs.EnsureJSON(raw, time.Now())
	if !bytes.Equal(got, raw) {
		t.Errorf("valid JSON was modified:\n  in:  %s\n  out: %s", raw, got)
	}
}

func TestEnsureJSON_WrapsNonJSON(t *testing.T) {
	raw := []byte("plain text startup banner")
	now := time.Unix(1234567890, 500_000_000) // 1234567890.5s
	got := logs.EnsureJSON(raw, now)
	if got == nil {
		t.Fatal("non-JSON input returned nil (expected wrapped)")
	}
	var parsed map[string]any
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("wrapped output is not valid JSON: %v\n  got: %s", err, got)
	}
	if parsed["level"] != "info" {
		t.Errorf("level: got %v, want info", parsed["level"])
	}
	if parsed["module"] != "sentinel-raw" {
		t.Errorf("module: got %v, want sentinel-raw", parsed["module"])
	}
	if parsed["msg"] != "plain text startup banner" {
		t.Errorf("msg: got %v, want original text", parsed["msg"])
	}
	ts, ok := parsed["ts"].(float64)
	if !ok {
		t.Fatalf("ts is not a float: got %T (%v)", parsed["ts"], parsed["ts"])
	}
	wantTs := 1234567890.5
	if ts < wantTs-0.01 || ts > wantTs+0.01 {
		t.Errorf("ts: got %v, want ~%v", ts, wantTs)
	}
}

func TestEnsureJSON_EmptyReturnsNil(t *testing.T) {
	for _, in := range []string{"", "   ", "\t\n"} {
		if got := logs.EnsureJSON([]byte(in), time.Now()); got != nil {
			t.Errorf("empty/whitespace %q: got %s, want nil", in, got)
		}
	}
}

func TestEnsureJSON_PreservesSpecialCharsInMsg(t *testing.T) {
	// Quotes, backslashes, newlines — must survive as valid JSON string.
	raw := []byte(`weird "quoted" and \backslash with \n escapes`)
	got := logs.EnsureJSON(raw, time.Now())
	var parsed map[string]any
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("wrap produced invalid JSON for special chars: %v", err)
	}
	if parsed["msg"].(string) != string(raw) {
		t.Errorf("msg roundtrip failed:\n  in:  %s\n  out: %s", raw, parsed["msg"])
	}
}

func TestEnsureJSON_PartialJSONIsWrapped(t *testing.T) {
	// A string that STARTS with { but isn't valid JSON should be wrapped, not passed through.
	raw := []byte(`{incomplete`)
	got := logs.EnsureJSON(raw, time.Now())
	var parsed map[string]any
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("wrap produced invalid JSON: %v", err)
	}
	if parsed["module"] != "sentinel-raw" {
		t.Errorf("expected wrapping; got module=%v", parsed["module"])
	}
}
