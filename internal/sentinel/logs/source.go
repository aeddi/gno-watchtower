package logs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// LogLine is a single log line from gnoland with the level pre-parsed for filtering.
// Raw contains the original JSON bytes and is included verbatim in the wire payload.
type LogLine struct {
	Level string
	Raw   json.RawMessage
}

// Source tails log output from a gnoland instance.
// Tail sends LogLines to out until ctx is cancelled.
type Source interface {
	Tail(ctx context.Context, out chan<- LogLine) error
}

// NewSource constructs a Source based on sourceType ("docker" or "journald").
// containerName is used for "docker"; unit is used for "journald".
// resumeLookback controls how far back the source reads on each (re)connect
// (docker only; journald has its own cursor mechanism).
func NewSource(sourceType, containerName, unit string, resumeLookback time.Duration) (Source, error) {
	switch sourceType {
	case "docker":
		return NewDockerSource(containerName, resumeLookback), nil
	case "journald":
		return NewJournaldSource(unit), nil
	default:
		return nil, fmt.Errorf("unknown log source %q: must be docker or journald", sourceType)
	}
}

// ParseLevel extracts the "level" field from a raw JSON log line.
// Returns "info" if the field is absent, invalid JSON, or not one of debug/info/warn/error.
func ParseLevel(raw json.RawMessage) string {
	var line struct {
		Level string `json:"level"`
	}
	if err := json.Unmarshal(raw, &line); err != nil {
		return "info"
	}
	switch line.Level {
	case "debug", "info", "warn", "error":
		return line.Level
	default:
		return "info"
	}
}

// EnsureJSON normalizes a raw log line so downstream consumers (Loki, Grafana)
// can rely on four invariants: the payload is a valid JSON object, and it
// always has "ts", "level", "msg", and "module" fields.
//
//   - Empty/whitespace input → nil (caller should skip).
//   - Non-JSON or JSON that isn't an object (e.g. `42`, `"x"`, `[…]`, `null`)
//     → wrapped as a synthetic line with module="sentinel-raw" and msg=original.
//   - Valid JSON object missing any mandatory field → missing fields filled
//     with defaults (ts=now, level="info", msg="", module="unknown").
//     module="unknown" is deliberately distinct from "sentinel-raw" so
//     dashboards can tell a fill-in apart from a non-JSON wrap.
//   - Valid JSON object with all 4 mandatory fields → returned unchanged.
func EnsureJSON(raw []byte, now time.Time) json.RawMessage {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if !json.Valid(raw) {
		return wrapAsSentinelRaw(raw, now)
	}
	// Valid JSON. Must also be an OBJECT to carry the mandatory fields.
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		return wrapAsSentinelRaw(raw, now)
	}
	// Fast path: all 4 mandatory fields present → no reserialization.
	if _, ok1 := obj["ts"]; ok1 {
		if _, ok2 := obj["level"]; ok2 {
			if _, ok3 := obj["msg"]; ok3 {
				if _, ok4 := obj["module"]; ok4 {
					return json.RawMessage(raw)
				}
			}
		}
	}
	// Fill missing mandatory fields while preserving original extras.
	if _, ok := obj["ts"]; !ok {
		obj["ts"] = json.RawMessage(strconv.FormatFloat(float64(now.UnixNano())/1e9, 'f', -1, 64))
	}
	if _, ok := obj["level"]; !ok {
		obj["level"] = json.RawMessage(`"info"`)
	}
	if _, ok := obj["msg"]; !ok {
		obj["msg"] = json.RawMessage(`""`)
	}
	if _, ok := obj["module"]; !ok {
		obj["module"] = json.RawMessage(`"unknown"`)
	}
	filled, err := json.Marshal(obj)
	if err != nil {
		return wrapAsSentinelRaw(raw, now)
	}
	return json.RawMessage(filled)
}

// wrapAsSentinelRaw produces a synthetic JSON line for input that isn't a
// valid JSON object. msg carries the original bytes as a UTF-8 string.
func wrapAsSentinelRaw(raw []byte, now time.Time) json.RawMessage {
	wrapped, err := json.Marshal(map[string]any{
		"level":  "info",
		"ts":     float64(now.UnixNano()) / 1e9,
		"module": "sentinel-raw",
		"msg":    string(raw),
	})
	if err != nil {
		// json.Marshal only fails on unsupported types — none here — but be
		// defensive: fall back to a minimal constant envelope.
		return json.RawMessage(`{"level":"info","module":"sentinel-raw","msg":"<unserializable>"}`)
	}
	return json.RawMessage(wrapped)
}
