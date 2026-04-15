package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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
func NewSource(sourceType, containerName, unit string, log *slog.Logger) (Source, error) {
	switch sourceType {
	case "docker":
		return NewDockerSource(containerName, log), nil
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

// NormalizeLogLine ensures raw is a valid JSON object suitable for the wire payload.
// If raw is already valid JSON it is returned unchanged (transformed = false).
// Otherwise the bytes are treated as plain text and wrapped into:
//
//	{"level":"info","msg":"<escaped text>"}
//
// transformed = true signals to callers that a conversion took place.
func NormalizeLogLine(raw []byte) (line json.RawMessage, transformed bool) {
	if json.Valid(raw) {
		return json.RawMessage(append([]byte(nil), raw...)), false
	}
	wrapped, _ := json.Marshal(map[string]string{
		"level": "info",
		"msg":   string(raw),
	})
	return json.RawMessage(wrapped), true
}
