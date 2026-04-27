package kinds

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/aeddi/gno-watchtower/internal/scribe/normalizer"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

// matchLog decodes a slog JSON line into a flat map and checks that msg contains
// the given marker. Returns the map and true when matched.
func matchLog(line, msgContains string) (map[string]any, bool) {
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &m); err != nil {
		return nil, false
	}
	msg, _ := m["msg"].(string)
	if !strings.Contains(msg, msgContains) {
		return nil, false
	}
	return m, true
}

// provenanceFromEntry builds a ProvenanceLog Provenance from a log observation.
func provenanceFromEntry(o normalizer.Observation) types.Provenance {
	stream := make(map[string]string, len(o.LogEntry.Stream.Labels))
	for k, v := range o.LogEntry.Stream.Labels {
		stream[k] = v
	}
	hash := sha1.Sum([]byte(o.LogEntry.Line))
	return types.Provenance{
		Type:  types.ProvenanceLog,
		Query: o.LogQuery,
		LogRefs: []types.LogRef{{
			StreamLabels: stream,
			LineTime:     o.LogEntry.Time,
			LineHash:     hex.EncodeToString(hash[:]),
		}},
	}
}

// readInt64 coerces a JSON-decoded field (float64 or int64) to int64.
func readInt64(m map[string]any, k string) int64 {
	switch v := m[k].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	default:
		return 0
	}
}

func readInt32(m map[string]any, k string) int32 { return int32(readInt64(m, k)) }
