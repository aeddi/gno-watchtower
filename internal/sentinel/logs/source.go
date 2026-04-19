package logs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

// EnsureJSON returns raw unchanged if it's already valid JSON; otherwise it
// wraps raw as a synthetic info-level line with module="sentinel-raw", so
// non-JSON stdout (e.g. gnoland startup banners emitted before the JSON logger
// is ready) is still observable in Loki instead of being silently dropped.
// Returns nil for empty/whitespace-only input — callers should skip those.
func EnsureJSON(raw []byte, now time.Time) json.RawMessage {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if json.Valid(raw) {
		return json.RawMessage(raw)
	}
	wrapped, err := json.Marshal(map[string]any{
		"level":  "info",
		"ts":     float64(now.UnixNano()) / 1e9,
		"module": "sentinel-raw",
		"msg":    string(raw),
	})
	if err != nil {
		// json.Marshal can fail only on unsupported types — none here — but
		// be defensive: fall back to a minimal constant envelope so we still
		// emit *something* for every line.
		return json.RawMessage(`{"level":"info","module":"sentinel-raw","msg":"<unserializable>"}`)
	}
	return json.RawMessage(wrapped)
}
