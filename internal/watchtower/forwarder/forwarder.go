package forwarder

import (
	"bytes"
	"cmp"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/klauspost/compress/zstd"
	collectorpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/proto"

	"github.com/aeddi/gno-watchtower/pkg/logger"
	"github.com/aeddi/gno-watchtower/pkg/protocol"
)

// Forwarder sends payloads to VictoriaMetrics and Loki.
type Forwarder struct {
	vmURL               string
	lokiURL             string
	client              *http.Client
	onLogsBelowMinLevel func(validator string)
}

// New creates a Forwarder. onLogsBelowMinLevel (may be nil) is invoked when
// a log payload is dropped because its level falls below the validator's
// configured logs_min_level — wire this to metrics.Metrics.RecordLogsBelowMinLevel
// so filtered traffic surfaces as watchtower_logs_below_min_level_total{validator}.
func New(vmURL, lokiURL string, onLogsBelowMinLevel func(validator string)) *Forwarder {
	return &Forwarder{
		vmURL:               vmURL,
		lokiURL:             lokiURL,
		client:              &http.Client{Timeout: 30 * time.Second},
		onLogsBelowMinLevel: onLogsBelowMinLevel,
	}
}

// vmLine is one entry in the VictoriaMetrics /api/v1/import JSON lines format.
type vmLine struct {
	Metric     map[string]string `json:"metric"`
	Values     []float64         `json:"values"`
	Timestamps []int64           `json:"timestamps"`
}

// postVMLines encodes lines as newline-delimited JSON and POSTs to /api/v1/import.
func (f *Forwarder) postVMLines(ctx context.Context, lines []vmLine) error {
	if len(lines) == 0 {
		return nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, l := range lines {
		if err := enc.Encode(l); err != nil {
			return fmt.Errorf("encode vm line: %w", err)
		}
	}
	return f.post(ctx, f.vmURL+"/api/v1/import", buf.Bytes(), "application/json")
}

// ForwardRPC decodes a sentinel RPC payload and forwards named Prometheus
// metrics (sentinel_rpc_*) to VictoriaMetrics. An empty payload (no changed
// endpoints from the sentinel's delta filter) is a no-op.
func (f *Forwarder) ForwardRPC(ctx context.Context, validator string, body []byte) error {
	var payload protocol.RPCPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("unmarshal rpc payload: %w", err)
	}
	return f.postVMLines(ctx, extractRPC(validator, payload))
}

// ForwardMetrics decodes a sentinel resource payload and forwards named
// Prometheus metrics (sentinel_host_* and sentinel_container_*) to VictoriaMetrics.
// An empty payload (no changed keys from the sentinel's delta filter) is a no-op.
func (f *Forwarder) ForwardMetrics(ctx context.Context, validator string, body []byte) error {
	var payload protocol.MetricsPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("unmarshal metrics payload: %w", err)
	}
	lines := extractMetrics(validator, payload)
	return f.postVMLines(ctx, lines)
}

// ForwardLogs decompresses the zstd-encoded LogPayload body and pushes it to Loki.
// If minLevel is non-empty and the payload level ranks below it, the payload is
// dropped silently (nil error). This is the server-side level filter; the sentinel
// also filters at source, so dropped payloads normally shouldn't reach here.
func (f *Forwarder) ForwardLogs(ctx context.Context, validator string, body []byte, minLevel string) error {
	decompressed, err := zstdDecompress(body)
	if err != nil {
		return fmt.Errorf("decompress logs: %w", err)
	}
	var payload protocol.LogPayload
	if err := json.Unmarshal(decompressed, &payload); err != nil {
		return fmt.Errorf("unmarshal log payload: %w", err)
	}
	if minLevel != "" && logger.LevelRank(payload.Level) < logger.LevelRank(minLevel) {
		if f.onLogsBelowMinLevel != nil {
			f.onLogsBelowMinLevel(validator)
		}
		return nil
	}
	push, err := toLokiPush(validator, payload)
	if err != nil {
		return fmt.Errorf("build loki push: %w", err)
	}
	b, err := json.Marshal(push)
	if err != nil {
		return fmt.Errorf("marshal loki push: %w", err)
	}
	return f.post(ctx, f.lokiURL+"/loki/api/v1/push", b, "application/json")
}

// ForwardOTLP injects the validator resource attribute and forwards protobuf to VictoriaMetrics.
// Automatically decompresses gzip-encoded bodies (the OTel Collector default).
func (f *Forwarder) ForwardOTLP(ctx context.Context, validator string, body []byte) error {
	// Auto-detect gzip compression (magic bytes 0x1f 0x8b).
	if len(body) >= 2 && body[0] == 0x1f && body[1] == 0x8b {
		gr, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("gzip reader: %w", err)
		}
		defer gr.Close()
		body, err = io.ReadAll(gr)
		if err != nil {
			return fmt.Errorf("gzip decompress: %w", err)
		}
	}

	var req collectorpb.ExportMetricsServiceRequest
	if err := proto.Unmarshal(body, &req); err != nil {
		return fmt.Errorf("unmarshal otlp: %w", err)
	}
	for _, rm := range req.ResourceMetrics {
		if rm.Resource == nil {
			rm.Resource = &resourcepb.Resource{}
		}
		rm.Resource.Attributes = append(rm.Resource.Attributes, &commonpb.KeyValue{
			Key: "validator",
			Value: &commonpb.AnyValue{
				Value: &commonpb.AnyValue_StringValue{StringValue: validator},
			},
		})
	}
	out, err := proto.Marshal(&req)
	if err != nil {
		return fmt.Errorf("marshal otlp: %w", err)
	}
	return f.post(ctx, f.vmURL+"/opentelemetry/v1/metrics", out, "application/x-protobuf")
}

// ---- helpers

type lokiPush struct {
	Streams []lokiStream `json:"streams"`
}

// lokiStream follows the Loki push API shape. Each value is a 3-element
// heterogeneous array: [ "<nanos>", "<log line>", { "key": "value", ... } ]
// where the third element is per-entry structured metadata. Using []any here
// lets encoding/json emit that mixed shape without a custom Marshaler.
type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]any           `json:"values"`
}

// Asymmetric clock-skew window for Loki-bound timestamps. Mirrors Loki's own
// ingester gates: reject_old_samples_max_age (past) and creation_grace_period
// (future). A sentinel whose clock drifts past either bound would have its
// whole batch rejected; we clamp to now instead.
const (
	maxLogsTsFutureSkew = 10 * time.Minute
	maxLogsTsPastSkew   = 168 * time.Hour
)

// toLokiPush converts a LogPayload to the Loki push API format.
//
// Timestamps come from each line's "ts" field (nanoseconds since epoch). Lines
// without a parseable "ts", or whose ts falls outside Loki's ingester gates
// (see maxLogsTsPastSkew / maxLogsTsFutureSkew), use the watchtower's receive
// time.
//
// All lines for a given (validator, level) share a single stream. The "module"
// field is emitted as *per-entry structured metadata*, not a stream label —
// gnoland has ~15 distinct modules and promoting them to labels would push
// stream cardinality toward validators × levels × modules, which Loki treats
// as anti-pattern. Queries filter via `| module="X"` at query time; requires
// `allow_structured_metadata: true` in loki-config.yml (our default).
//
// Missing/empty module falls back to "unknown"; sentinel guarantees non-JSON
// stdout arrives here tagged as module="sentinel-raw".
func toLokiPush(validator string, payload protocol.LogPayload) (*lokiPush, error) {
	now := time.Now()
	type entry struct {
		nano   int64
		raw    string
		module string
	}
	entries := make([]entry, 0, len(payload.Lines))
	for _, raw := range payload.Lines {
		mod, ts := extractModuleAndTS(raw, now)
		if ts.Before(now.Add(-maxLogsTsPastSkew)) || ts.After(now.Add(maxLogsTsFutureSkew)) {
			ts = now
		}
		// "ts" and "level" are promoted: ts becomes the Loki entry timestamp,
		// level becomes a stream label below. Leaving them in the body makes
		// Grafana surface them as noisy duplicates ("ts" chip,
		// "level_extracted" alongside the real level label).
		entries = append(entries, entry{nano: ts.UnixNano(), raw: string(stripBodyFields(raw, "ts", "level")), module: mod})
	}
	// Loki rejects out-of-order entries per stream with 400. Upstream sentinel
	// batches preserve order from docker, but sort defensively. Sort
	// numerically on the parsed nano rather than the formatted string because
	// future sub-1970 timestamps would sort incorrectly as strings.
	slices.SortFunc(entries, func(a, b entry) int { return cmp.Compare(a.nano, b.nano) })
	values := make([][]any, len(entries))
	for i, e := range entries {
		values[i] = []any{
			strconv.FormatInt(e.nano, 10),
			e.raw,
			map[string]string{"module": e.module},
		}
	}
	if len(values) == 0 {
		return &lokiPush{Streams: []lokiStream{}}, nil
	}
	return &lokiPush{Streams: []lokiStream{{
		Stream: map[string]string{
			"validator": validator,
			"level":     payload.Level,
		},
		Values: values,
	}}}, nil
}

// extractModuleAndTS reads both the "module" and "ts" fields from a raw JSON
// line in a single Unmarshal call. Module defaults to "unknown" when missing/
// empty; timestamp falls back to the caller-provided time when absent or
// unparseable. "ts" is accepted as either a JSON number (epoch seconds,
// possibly fractional — gnoland/zap format) or an RFC3339/RFC3339Nano string.
func extractModuleAndTS(raw json.RawMessage, fallback time.Time) (string, time.Time) {
	var line struct {
		Module string          `json:"module"`
		TS     json.RawMessage `json:"ts"`
	}
	if err := json.Unmarshal(raw, &line); err != nil {
		return "unknown", fallback
	}
	mod := line.Module
	if mod == "" {
		mod = "unknown"
	}
	ts := fallback
	if len(line.TS) > 0 {
		var epoch float64
		if err := json.Unmarshal(line.TS, &epoch); err == nil {
			sec := int64(epoch)
			nsec := int64((epoch - float64(sec)) * 1e9)
			ts = time.Unix(sec, nsec)
		} else {
			var s string
			if err := json.Unmarshal(line.TS, &s); err == nil && s != "" {
				if parsed, err := time.Parse(time.RFC3339Nano, s); err == nil {
					ts = parsed
				}
			}
		}
	}
	return mod, ts
}

// stripBodyFields removes top-level keys from a JSON-object body and returns
// the re-marshaled result. Used to drop fields that have been promoted out
// of the body (ts → entry timestamp, level → stream label) so Grafana doesn't
// surface them as duplicate chips. Non-object input is passed through
// untouched; downstream readers see the original bytes.
func stripBodyFields(raw json.RawMessage, keys ...string) json.RawMessage {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		return raw
	}
	for _, k := range keys {
		delete(obj, k)
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return raw
	}
	return out
}

func (f *Forwarder) post(ctx context.Context, url string, body []byte, contentType string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := f.client.Do(req)
	if err != nil {
		return fmt.Errorf("post %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		excerpt, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		_, _ = io.Copy(io.Discard, resp.Body)
		if len(bytes.TrimSpace(excerpt)) > 0 {
			return fmt.Errorf("post %s: status %d: %s", url, resp.StatusCode, bytes.TrimSpace(excerpt))
		}
		return fmt.Errorf("post %s: status %d", url, resp.StatusCode)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// maxLogsDecompressedBytes caps the expanded size of a sentinel log payload.
// zstd bombs can expand compressed input 100× or more; we accept up to 50 MiB
// compressed (see handlers.maxBodyBytes) so bound the decompressed side at
// ~10× that — generous for legitimate debug-mode batches, hostile to zip bombs.
const maxLogsDecompressedBytes = 500 << 20

// zstdDecompress decompresses zstd-encoded bytes with a hard cap on output
// size. A new Decoder is built per call (stateful stream readers aren't safe
// for concurrent reuse); the per-call init cost is negligible next to the
// network round-trip the payload already traversed.
func zstdDecompress(data []byte) ([]byte, error) {
	dec, err := zstd.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("zstd: %w", err)
	}
	defer dec.Close()
	// Read one byte past the ceiling so we can distinguish "exactly the limit"
	// from "tried to exceed the limit".
	out, err := io.ReadAll(io.LimitReader(dec, maxLogsDecompressedBytes+1))
	if err != nil {
		return nil, fmt.Errorf("zstd read: %w", err)
	}
	if int64(len(out)) > maxLogsDecompressedBytes {
		return nil, fmt.Errorf("zstd: decompressed payload exceeds %d bytes", maxLogsDecompressedBytes)
	}
	return out, nil
}
