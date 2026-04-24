package forwarder_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
	collectorpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/proto"

	"github.com/aeddi/gno-watchtower/internal/watchtower/forwarder"
	"github.com/aeddi/gno-watchtower/pkg/protocol"
)

func TestForwardRPC_PostsSentinelMetricsToVM(t *testing.T) {
	var received []byte
	var path string
	vmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		received, _ = io.ReadAll(r.Body)
	}))
	defer vmSrv.Close()

	payload := protocol.RPCPayload{
		CollectedAt: time.Now().UTC(),
		Data: map[string]json.RawMessage{
			"net_info": json.RawMessage(`{"n_peers":"7"}`),
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	f := forwarder.New(vmSrv.URL, "http://loki-unused:3100", nil)
	if err := f.ForwardRPC(context.Background(), "val-01", body); err != nil {
		t.Fatalf("ForwardRPC: %v", err)
	}
	if path != "/api/v1/import" {
		t.Errorf("VM path = %q, want /api/v1/import", path)
	}
	if !strings.Contains(string(received), `"sentinel_rpc_peers"`) {
		t.Errorf("VM body missing sentinel_rpc_peers: %s", received)
	}
	if !strings.Contains(string(received), `"val-01"`) {
		t.Errorf("VM body missing validator label: %s", received)
	}
}

func TestForwardRPC_EmptyPayloadNoPost(t *testing.T) {
	var called bool
	vmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer vmSrv.Close()
	f := forwarder.New(vmSrv.URL, "http://loki-unused:3100", nil)
	body := []byte(`{"collected_at":"2026-04-19T12:00:00Z","data":{}}`)
	if err := f.ForwardRPC(context.Background(), "val-01", body); err != nil {
		t.Fatalf("ForwardRPC: %v", err)
	}
	if called {
		t.Error("empty payload should not POST to VM")
	}
}

func TestForwardLogs_DropsWhenPayloadLevelBelowMinLevel(t *testing.T) {
	var called bool
	lokiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer lokiSrv.Close()

	payload := protocol.LogPayload{
		Level: "debug",
		Lines: []json.RawMessage{json.RawMessage(`{"level":"debug","msg":"noisy"}`)},
	}
	body, _ := json.Marshal(payload)
	compressed, _ := zstdCompress(body)

	var droppedFor string
	onDrop := func(validator string) { droppedFor = validator }
	f := forwarder.New("http://vm-unused:8428", lokiSrv.URL, onDrop)
	if err := f.ForwardLogs(context.Background(), "val-01", compressed, "warn"); err != nil {
		t.Fatalf("ForwardLogs: %v", err)
	}
	if called {
		t.Error("Loki was called but payload should have been dropped (debug < warn)")
	}
	if droppedFor != "val-01" {
		t.Errorf("onDrop callback: got %q, want \"val-01\"", droppedFor)
	}
}

func TestForwardLogs_ForwardsWhenPayloadLevelAtOrAboveMinLevel(t *testing.T) {
	var called bool
	lokiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer lokiSrv.Close()

	payload := protocol.LogPayload{
		Level: "error",
		Lines: []json.RawMessage{json.RawMessage(`{"level":"error","msg":"boom"}`)},
	}
	body, _ := json.Marshal(payload)
	compressed, _ := zstdCompress(body)

	f := forwarder.New("http://vm-unused:8428", lokiSrv.URL, nil)
	if err := f.ForwardLogs(context.Background(), "val-01", compressed, "warn"); err != nil {
		t.Fatalf("ForwardLogs: %v", err)
	}
	if !called {
		t.Error("Loki was not called but payload should have been forwarded (error >= warn)")
	}
}

func TestForwardLogs_UsesGnoFloatEpochTimestamp(t *testing.T) {
	var lokiBody []byte
	lokiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lokiBody, _ = io.ReadAll(r.Body)
	}))
	defer lokiSrv.Close()

	// gnoland emits `ts` as a JSON number (seconds since epoch with fractional part).
	payload := protocol.LogPayload{
		Level: "info",
		Lines: []json.RawMessage{
			json.RawMessage(`{"level":"info","ts":1776544197.2256565,"msg":"hi"}`),
		},
	}
	body, _ := json.Marshal(payload)
	compressed, _ := zstdCompress(body)

	f := forwarder.New("http://vm-unused:8428", lokiSrv.URL, nil)
	if err := f.ForwardLogs(context.Background(), "val-01", compressed, ""); err != nil {
		t.Fatalf("ForwardLogs: %v", err)
	}

	var push struct {
		Streams []struct {
			Stream map[string]string   `json:"stream"`
			Values [][]json.RawMessage `json:"values"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(lokiBody, &push); err != nil {
		t.Fatalf("parse loki body: %v", err)
	}
	if len(push.Streams) == 0 || len(push.Streams[0].Values) == 0 {
		t.Fatal("no values pushed to loki")
	}
	// Each value is [ "<nanos>", "<line>", {metadata} ]; we only need [0].
	var gotTS string
	if err := json.Unmarshal(push.Streams[0].Values[0][0], &gotTS); err != nil {
		t.Fatalf("unmarshal ts: %v", err)
	}
	// 1776544197.2256565 seconds → 1776544197225656500 ns. Allow ±1ms rounding slack.
	const wantNs = int64(1776544197225656500)
	var got int64
	if _, err := fmt.Sscanf(gotTS, "%d", &got); err != nil {
		t.Fatalf("parse pushed ts %q: %v", gotTS, err)
	}
	delta := got - wantNs
	if delta < -int64(time.Millisecond) || delta > int64(time.Millisecond) {
		t.Errorf("loki ts %d off from gno ts %d by %s (want: use gno `ts`, not push-time fallback)",
			got, wantNs, time.Duration(delta))
	}
}

func TestForwardLogs_EmptyMinLevelMeansNoFilter(t *testing.T) {
	var called bool
	lokiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer lokiSrv.Close()

	payload := protocol.LogPayload{
		Level: "debug",
		Lines: []json.RawMessage{json.RawMessage(`{"level":"debug","msg":"hi"}`)},
	}
	body, _ := json.Marshal(payload)
	compressed, _ := zstdCompress(body)

	f := forwarder.New("http://vm-unused:8428", lokiSrv.URL, nil)
	if err := f.ForwardLogs(context.Background(), "val-01", compressed, ""); err != nil {
		t.Fatalf("ForwardLogs: %v", err)
	}
	if !called {
		t.Error("Loki was not called but empty minLevel should disable filtering")
	}
}

// TestForwardLogs_StripsPromotedFieldsFromBody asserts the JSON body pushed
// to Loki no longer carries "ts" or "level" — both are promoted elsewhere
// (ts → Loki entry timestamp; level → stream label). Keeping them in the
// body yields noise in Grafana: a "ts" chip and a "level_extracted" field
// alongside the real level label.
func TestForwardLogs_StripsPromotedFieldsFromBody(t *testing.T) {
	var lokiBody []byte
	lokiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lokiBody, _ = io.ReadAll(r.Body)
	}))
	defer lokiSrv.Close()

	payload := protocol.LogPayload{
		Level: "info",
		Lines: []json.RawMessage{
			json.RawMessage(`{"ts":1776544197.2256565,"level":"info","msg":"hi","module":"foo","caller":"x.go:1"}`),
		},
	}
	body, _ := json.Marshal(payload)
	compressed, _ := zstdCompress(body)

	f := forwarder.New("http://vm-unused:8428", lokiSrv.URL, nil)
	if err := f.ForwardLogs(context.Background(), "val-01", compressed, ""); err != nil {
		t.Fatalf("ForwardLogs: %v", err)
	}

	var push struct {
		Streams []struct {
			Stream map[string]string   `json:"stream"`
			Values [][]json.RawMessage `json:"values"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(lokiBody, &push); err != nil {
		t.Fatalf("parse loki body: %v", err)
	}
	if len(push.Streams) == 0 || len(push.Streams[0].Values) == 0 {
		t.Fatal("no values pushed to loki")
	}

	var pushedLine string
	if err := json.Unmarshal(push.Streams[0].Values[0][1], &pushedLine); err != nil {
		t.Fatalf("unmarshal line body: %v", err)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(pushedLine), &obj); err != nil {
		t.Fatalf("parse line body %q: %v", pushedLine, err)
	}
	if _, has := obj["ts"]; has {
		t.Errorf(`"ts" should have been stripped from the body; got %q`, pushedLine)
	}
	if _, has := obj["level"]; has {
		t.Errorf(`"level" should have been stripped from the body; got %q`, pushedLine)
	}
	for _, k := range []string{"msg", "module", "caller"} {
		if _, has := obj[k]; !has {
			t.Errorf("body should have preserved %q; got %q", k, pushedLine)
		}
	}
}

func TestForwardMetrics_PostsSentinelMetricsToVM(t *testing.T) {
	var received []byte
	var path string
	vmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		received, _ = io.ReadAll(r.Body)
	}))
	defer vmSrv.Close()

	payload := protocol.MetricsPayload{
		CollectedAt: time.Now().UTC(),
		Data:        map[string]json.RawMessage{"cpu": json.RawMessage(`[33.0]`)},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	f := forwarder.New(vmSrv.URL, "http://loki-unused:3100", nil)
	if err := f.ForwardMetrics(context.Background(), "val-01", body); err != nil {
		t.Fatalf("ForwardMetrics: %v", err)
	}
	if path != "/api/v1/import" {
		t.Errorf("VM path = %q, want /api/v1/import", path)
	}
	// The VM /api/v1/import body is newline-delimited JSON; we posted one sample.
	var line struct {
		Metric     map[string]string `json:"metric"`
		Values     []float64         `json:"values"`
		Timestamps []int64           `json:"timestamps"`
	}
	dec := json.NewDecoder(bytes.NewReader(received))
	if err := dec.Decode(&line); err != nil {
		t.Fatalf("decode VM body: %v (body=%q)", err, received)
	}
	if line.Metric["__name__"] != "sentinel_host_cpu_percent" {
		t.Errorf("__name__ = %q, want sentinel_host_cpu_percent", line.Metric["__name__"])
	}
	if line.Metric["validator"] != "val-01" {
		t.Errorf("validator = %q, want val-01", line.Metric["validator"])
	}
	if len(line.Values) != 1 || line.Values[0] != 33.0 {
		t.Errorf("values = %v, want [33]", line.Values)
	}
}

func TestForwardMetrics_EmptyPayloadNoPost(t *testing.T) {
	var called bool
	vmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer vmSrv.Close()

	f := forwarder.New(vmSrv.URL, "http://loki-unused:3100", nil)
	body := []byte(`{"collected_at":"2026-04-19T12:00:00Z","data":{}}`)
	if err := f.ForwardMetrics(context.Background(), "val-01", body); err != nil {
		t.Fatalf("ForwardMetrics: %v", err)
	}
	if called {
		t.Error("empty payload should not POST to VM")
	}
}

func TestForwardLogs_DecompressesAndPushesToLoki(t *testing.T) {
	var lokiBody []byte
	lokiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lokiBody, _ = io.ReadAll(r.Body)
		if r.URL.Path != "/loki/api/v1/push" {
			t.Errorf("unexpected loki path: %s", r.URL.Path)
		}
	}))
	defer lokiSrv.Close()

	payload := protocol.LogPayload{
		Level: "warn",
		Lines: []json.RawMessage{
			json.RawMessage(`{"level":"warn","msg":"test","ts":"2026-01-01T00:00:01Z","module":"rpc-server"}`),
		},
	}
	body, _ := json.Marshal(payload)
	compressed, _ := zstdCompress(body)

	f := forwarder.New("http://vm-unused:8428", lokiSrv.URL, nil)
	if err := f.ForwardLogs(context.Background(), "val-01", compressed, ""); err != nil {
		t.Fatalf("ForwardLogs: %v", err)
	}

	var push struct {
		Streams []struct {
			Stream map[string]string   `json:"stream"`
			Values [][]json.RawMessage `json:"values"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(lokiBody, &push); err != nil {
		t.Fatalf("parse loki body: %v — raw: %s", err, lokiBody)
	}
	if len(push.Streams) == 0 {
		t.Fatal("no streams in loki push")
	}
	s := push.Streams[0]
	if s.Stream["validator"] != "val-01" {
		t.Errorf("validator label: got %q", s.Stream["validator"])
	}
	if s.Stream["level"] != "warn" {
		t.Errorf("level label: got %q", s.Stream["level"])
	}
	// module must NOT be a stream label — cardinality concern. It goes in
	// per-entry structured metadata instead.
	if _, ok := s.Stream["module"]; ok {
		t.Errorf("module promoted to stream label (should be structured metadata): %v", s.Stream)
	}
	if len(s.Values) == 0 {
		t.Fatal("no values in stream")
	}
	var md map[string]string
	if err := json.Unmarshal(s.Values[0][2], &md); err != nil {
		t.Fatalf("unmarshal structured metadata: %v", err)
	}
	if md["module"] != "rpc-server" {
		t.Errorf("module metadata: got %q (want rpc-server)", md["module"])
	}
}

func TestForwardLogs_MissingModuleFallsBackToUnknown(t *testing.T) {
	// Defensive: if a line somehow reaches watchtower without a module field
	// (pre-EnsureJSON data, or a broken producer), route it to module="unknown"
	// rather than silently dropping the module label.
	var lokiBody []byte
	lokiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lokiBody, _ = io.ReadAll(r.Body)
	}))
	defer lokiSrv.Close()

	payload := protocol.LogPayload{
		Level: "info",
		Lines: []json.RawMessage{
			json.RawMessage(`{"level":"info","ts":1.0,"msg":"no module here"}`),
		},
	}
	body, _ := json.Marshal(payload)
	compressed, _ := zstdCompress(body)

	f := forwarder.New("http://vm-unused:8428", lokiSrv.URL, nil)
	if err := f.ForwardLogs(context.Background(), "val-01", compressed, ""); err != nil {
		t.Fatalf("ForwardLogs: %v", err)
	}
	var push struct {
		Streams []struct {
			Stream map[string]string   `json:"stream"`
			Values [][]json.RawMessage `json:"values"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(lokiBody, &push); err != nil {
		t.Fatalf("parse loki body: %v", err)
	}
	if len(push.Streams) != 1 || len(push.Streams[0].Values) != 1 {
		t.Fatalf("expected 1 stream with 1 value, got %+v", push)
	}
	var md map[string]string
	if err := json.Unmarshal(push.Streams[0].Values[0][2], &md); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if md["module"] != "unknown" {
		t.Errorf("missing module metadata: got %q, want 'unknown'", md["module"])
	}
}

func TestForwardOTLP_InjectsValidatorAndPostsToVM(t *testing.T) {
	var received []byte
	vmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
	}))
	defer vmSrv.Close()

	// Build an ExportMetricsServiceRequest with no resource attributes.
	req := &collectorpb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{
			{Resource: &resourcepb.Resource{}},
		},
	}
	body, _ := proto.Marshal(req)

	f := forwarder.New(vmSrv.URL, "http://loki-unused:3100", nil)
	if err := f.ForwardOTLP(context.Background(), "val-01", body); err != nil {
		t.Fatalf("ForwardOTLP: %v", err)
	}

	// The received bytes should decode to a request with validator attribute injected.
	var got collectorpb.ExportMetricsServiceRequest
	if err := proto.Unmarshal(received, &got); err != nil {
		t.Fatalf("unmarshal received: %v", err)
	}
	if len(got.ResourceMetrics) == 0 {
		t.Fatal("no resource metrics in forwarded payload")
	}
	attrs := got.ResourceMetrics[0].Resource.GetAttributes()
	var found bool
	for _, a := range attrs {
		if a.Key == "validator" {
			found = true
			if a.Value.GetStringValue() != "val-01" {
				t.Errorf("validator attr value: got %q", a.Value.GetStringValue())
			}
		}
	}
	if !found {
		t.Error("validator attribute not injected")
	}
}

func TestForwardLogs_InvalidCompression_ReturnsError(t *testing.T) {
	f := forwarder.New("http://vm:8428", "http://loki:3100", nil)
	err := f.ForwardLogs(context.Background(), "val-01", []byte("not zstd compressed"), "")
	if err == nil {
		t.Error("expected error for invalid zstd data")
	}
}

func TestForwardRPC_NonOKIncludesBody(t *testing.T) {
	vmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("entry out of order"))
	}))
	defer vmSrv.Close()

	payload := protocol.RPCPayload{
		CollectedAt: time.Now().UTC(),
		Data:        map[string]json.RawMessage{"net_info": json.RawMessage(`{"n_peers":"1"}`)},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	f := forwarder.New(vmSrv.URL, "http://loki-unused:3100", nil)
	err = f.ForwardRPC(context.Background(), "val-01", body)
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if !strings.Contains(err.Error(), "entry out of order") {
		t.Errorf("error should contain upstream body, got: %v", err)
	}
}

func zstdCompress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := zstd.NewWriter(&buf)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	w.Close()
	return buf.Bytes(), nil
}

// TestForwardLogs_DecompressedBombRejected exercises the zstd-bomb guard: a
// small compressed payload that expands past maxLogsDecompressedBytes (500 MiB)
// must be rejected with an error, not OOM the process.
func TestForwardLogs_DecompressedBombRejected(t *testing.T) {
	// Compress ~600 MiB of zeros — zstd collapses this to a few hundred bytes.
	payload := bytes.Repeat([]byte{0}, 600<<20)
	compressed, err := zstdCompress(payload)
	if err != nil {
		t.Fatalf("zstdCompress bomb: %v", err)
	}
	if len(compressed) > 1<<20 {
		t.Fatalf("sanity: bomb compressed too large (%d bytes)", len(compressed))
	}

	// A Loki server that should never be called.
	loki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Loki must not be called when decompression exceeds the cap")
	}))
	defer loki.Close()

	f := forwarder.New("http://vm-unused:8428", loki.URL, nil)
	err = f.ForwardLogs(context.Background(), "val-01", compressed, "")
	if err == nil {
		t.Fatal("expected ForwardLogs to reject oversized decompressed payload")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error should mention size ceiling; got: %v", err)
	}
}

// TestForwardLogs_ClockSkewClampedToNow covers the creation_grace_period gate:
// a sentinel with a wildly off clock would have its batch rejected by Loki; we
// clamp obviously-skewed timestamps to the watchtower's own clock before push.
func TestForwardLogs_ClockSkewClampedToNow(t *testing.T) {
	var lokiBody []byte
	loki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lokiBody, _ = io.ReadAll(r.Body)
	}))
	defer loki.Close()

	// Line carries a ts 1 hour in the future — well past the 10-min guard.
	farFuture := time.Now().Add(1 * time.Hour).Unix()
	line := fmt.Sprintf(`{"level":"info","ts":%d,"msg":"hello from the future","module":"rpc-server"}`, farFuture)
	payload := protocol.LogPayload{
		Level: "info",
		Lines: []json.RawMessage{json.RawMessage(line)},
	}
	body, _ := json.Marshal(payload)
	compressed, _ := zstdCompress(body)

	before := time.Now()
	f := forwarder.New("http://vm-unused:8428", loki.URL, nil)
	if err := f.ForwardLogs(context.Background(), "val-01", compressed, ""); err != nil {
		t.Fatalf("ForwardLogs: %v", err)
	}
	after := time.Now()

	var push struct {
		Streams []struct {
			Stream map[string]string   `json:"stream"`
			Values [][]json.RawMessage `json:"values"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(lokiBody, &push); err != nil {
		t.Fatalf("parse loki body: %v", err)
	}
	if len(push.Streams) != 1 || len(push.Streams[0].Values) != 1 {
		t.Fatalf("expected 1 stream with 1 value, got %+v", push)
	}
	var tsStr string
	if err := json.Unmarshal(push.Streams[0].Values[0][0], &tsStr); err != nil {
		t.Fatalf("unmarshal ts: %v", err)
	}
	gotNano, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		t.Fatalf("parse ts: %v", err)
	}
	got := time.Unix(0, gotNano)
	if got.Before(before) || got.After(after) {
		t.Errorf("ts not clamped to now: got %v, want between %v and %v", got, before, after)
	}
}
