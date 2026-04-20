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
	vmURL   string
	lokiURL string
	client  *http.Client
}

// New creates a Forwarder.
func New(vmURL, lokiURL string) *Forwarder {
	return &Forwarder{
		vmURL:   vmURL,
		lokiURL: lokiURL,
		client:  &http.Client{Timeout: 30 * time.Second},
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

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

// toLokiPush converts a LogPayload to the Loki push API format.
// Timestamps are extracted from the "ts" field of each line (nanoseconds since epoch).
// Lines without a parseable "ts" field use the current time.
//
// Lines are split across streams by their "module" field so that `module`
// becomes an indexed Loki label (required for Grafana's label_values(module)
// dropdown — structured metadata extracted at query time via `| json` doesn't
// populate the index). Missing/empty module falls back to "unknown"; sentinel
// guarantees non-JSON stdout arrives here tagged as module="sentinel-raw".
func toLokiPush(validator string, payload protocol.LogPayload) (*lokiPush, error) {
	now := time.Now()
	byModule := make(map[string][][]string)
	for _, raw := range payload.Lines {
		mod, ts := extractModuleAndTS(raw, now)
		byModule[mod] = append(byModule[mod], []string{
			strconv.FormatInt(ts.UnixNano(), 10),
			string(raw),
		})
	}
	streams := make([]lokiStream, 0, len(byModule))
	for mod, values := range byModule {
		// Loki rejects out-of-order entries per stream with 400. Upstream
		// sentinel batches preserve order from docker, but sort defensively so
		// a future reordering upstream can't silently break ingestion.
		slices.SortFunc(values, func(a, b []string) int { return cmp.Compare(a[0], b[0]) })
		streams = append(streams, lokiStream{
			Stream: map[string]string{
				"validator": validator,
				"level":     payload.Level,
				"module":    mod,
			},
			Values: values,
		})
	}
	return &lokiPush{Streams: streams}, nil
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

// zstdDecoder is a reusable stateless decoder; DecodeAll is safe for concurrent use.
var zstdDecoder = func() *zstd.Decoder {
	dec, err := zstd.NewReader(nil)
	if err != nil {
		panic("init zstd decoder: " + err.Error())
	}
	return dec
}()

func zstdDecompress(data []byte) ([]byte, error) {
	return zstdDecoder.DecodeAll(data, nil)
}
