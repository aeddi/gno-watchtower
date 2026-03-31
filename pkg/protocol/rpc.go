package protocol

import (
	"encoding/json"
	"time"
)

// RPCPayload is the body of POST /rpc.
// Data contains only endpoints whose response changed since last send (hash-based delta).
// Absent keys mean the endpoint response is unchanged.
type RPCPayload struct {
	CollectedAt time.Time                  `json:"collected_at"`
	Data        map[string]json.RawMessage `json:"data"`
}
