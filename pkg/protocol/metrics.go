// pkg/protocol/metrics.go
package protocol

import (
	"encoding/json"
	"time"
)

// MetricsPayload is the body of POST /metrics.
// Data contains only keys whose value changed since last send (hash-based delta).
// Absent keys mean the value is unchanged.
type MetricsPayload struct {
	CollectedAt time.Time                  `json:"collected_at"`
	Data        map[string]json.RawMessage `json:"data"`
}
