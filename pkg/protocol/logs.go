package protocol

import "encoding/json"

// LogPayload is the body of POST /logs (after zstd decompression).
// One payload per level per flush. Lines contains the raw JSON log objects from gnoland.
type LogPayload struct {
	Level string            `json:"level"`
	Lines []json.RawMessage `json:"lines"`
}
