package logs_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/sentinel/logs"
)

func TestEnsureJSON_PassesThroughValidJSONWithAllMandatoryFields(t *testing.T) {
	// All 4 mandatory fields (ts, level, msg, module) present → no modification.
	raw := []byte(`{"level":"info","ts":1234567890.5,"msg":"hello","module":"rpc-server","extra":42}`)
	got := logs.EnsureJSON(raw, time.Now())
	if !bytes.Equal(got, raw) {
		t.Errorf("fully-populated JSON was modified:\n  in:  %s\n  out: %s", raw, got)
	}
}

func TestEnsureJSON_FillsMissingLevel(t *testing.T) {
	raw := []byte(`{"ts":1.0,"msg":"x","module":"m"}`)
	got := logs.EnsureJSON(raw, time.Now())
	var p map[string]any
	if err := json.Unmarshal(got, &p); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	if p["level"] != "info" {
		t.Errorf("missing level should default to info; got %v", p["level"])
	}
}

func TestEnsureJSON_FillsMissingTs(t *testing.T) {
	raw := []byte(`{"level":"warn","msg":"x","module":"m"}`)
	now := time.Unix(1234567890, 500_000_000)
	got := logs.EnsureJSON(raw, now)
	var p map[string]any
	if err := json.Unmarshal(got, &p); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	ts, ok := p["ts"].(float64)
	if !ok {
		t.Fatalf("ts not a float: got %T (%v)", p["ts"], p["ts"])
	}
	if ts < 1234567890.4 || ts > 1234567890.6 {
		t.Errorf("ts: got %v, want ~1234567890.5", ts)
	}
}

func TestEnsureJSON_FillsMissingMsg(t *testing.T) {
	raw := []byte(`{"level":"info","ts":1.0,"module":"m"}`)
	got := logs.EnsureJSON(raw, time.Now())
	var p map[string]any
	if err := json.Unmarshal(got, &p); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	if p["msg"] != "" {
		t.Errorf("missing msg should default to empty string; got %v", p["msg"])
	}
}

func TestEnsureJSON_FillsMissingModule(t *testing.T) {
	raw := []byte(`{"level":"info","ts":1.0,"msg":"x"}`)
	got := logs.EnsureJSON(raw, time.Now())
	var p map[string]any
	if err := json.Unmarshal(got, &p); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	if p["module"] != "unknown" {
		t.Errorf("missing module should default to 'unknown' (distinct from sentinel-raw); got %v", p["module"])
	}
}

func TestEnsureJSON_FillsMultipleMissingFields(t *testing.T) {
	// Minimal JSON — only one field present. All others get filled.
	raw := []byte(`{"msg":"hi there"}`)
	got := logs.EnsureJSON(raw, time.Now())
	var p map[string]any
	if err := json.Unmarshal(got, &p); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	if p["level"] != "info" {
		t.Errorf("level: got %v", p["level"])
	}
	if p["module"] != "unknown" {
		t.Errorf("module: got %v", p["module"])
	}
	if _, ok := p["ts"].(float64); !ok {
		t.Errorf("ts: got %T (%v)", p["ts"], p["ts"])
	}
	// Existing msg must not be overwritten.
	if p["msg"] != "hi there" {
		t.Errorf("msg: got %v, want original 'hi there'", p["msg"])
	}
}

func TestEnsureJSON_WrapsValidJSONThatIsNotAnObject(t *testing.T) {
	// A JSON number, string, or array is technically valid JSON but can't have
	// fields added. Treat as raw and wrap.
	for _, in := range []string{`42`, `"string literal"`, `[1,2,3]`, `null`} {
		got := logs.EnsureJSON([]byte(in), time.Now())
		if got == nil {
			t.Errorf("got nil for %q, want wrapped", in)
			continue
		}
		var p map[string]any
		if err := json.Unmarshal(got, &p); err != nil {
			t.Errorf("not a wrapped JSON object for %q: %v", in, err)
			continue
		}
		if p["module"] != "sentinel-raw" {
			t.Errorf("input %q: expected sentinel-raw wrap, got module=%v", in, p["module"])
		}
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
	if parsed["level"] != "warn" {
		t.Errorf("level: got %v, want warn (sentinel-raw is abnormal output)", parsed["level"])
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
