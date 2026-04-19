package forwarder_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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

func TestForwardRPC_PostsToVM(t *testing.T) {
	var received []byte
	var path string
	vmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		received, _ = io.ReadAll(r.Body)
	}))
	defer vmSrv.Close()

	f := forwarder.New(vmSrv.URL, "http://loki-unused:3100")
	if err := f.ForwardRPC(context.Background(), "val-01", []byte(`{}`)); err != nil {
		t.Fatalf("ForwardRPC: %v", err)
	}
	if len(received) == 0 {
		t.Error("VM received nothing")
	}
	if path != "/api/v1/import" {
		t.Errorf("unexpected VM path: %q (want /api/v1/import)", path)
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

	f := forwarder.New("http://vm-unused:8428", lokiSrv.URL)
	if err := f.ForwardLogs(context.Background(), "val-01", compressed, "warn"); err != nil {
		t.Fatalf("ForwardLogs: %v", err)
	}
	if called {
		t.Error("Loki was called but payload should have been dropped (debug < warn)")
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

	f := forwarder.New("http://vm-unused:8428", lokiSrv.URL)
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

	f := forwarder.New("http://vm-unused:8428", lokiSrv.URL)
	if err := f.ForwardLogs(context.Background(), "val-01", compressed, ""); err != nil {
		t.Fatalf("ForwardLogs: %v", err)
	}

	var push struct {
		Streams []struct {
			Stream map[string]string `json:"stream"`
			Values [][]string        `json:"values"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(lokiBody, &push); err != nil {
		t.Fatalf("parse loki body: %v", err)
	}
	if len(push.Streams) == 0 || len(push.Streams[0].Values) == 0 {
		t.Fatal("no values pushed to loki")
	}
	gotTS := push.Streams[0].Values[0][0]
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

	f := forwarder.New("http://vm-unused:8428", lokiSrv.URL)
	if err := f.ForwardLogs(context.Background(), "val-01", compressed, ""); err != nil {
		t.Fatalf("ForwardLogs: %v", err)
	}
	if !called {
		t.Error("Loki was not called but empty minLevel should disable filtering")
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
	f := forwarder.New(vmSrv.URL, "http://loki-unused:3100")
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

	f := forwarder.New(vmSrv.URL, "http://loki-unused:3100")
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

	f := forwarder.New("http://vm-unused:8428", lokiSrv.URL)
	if err := f.ForwardLogs(context.Background(), "val-01", compressed, ""); err != nil {
		t.Fatalf("ForwardLogs: %v", err)
	}

	var push struct {
		Streams []struct {
			Stream map[string]string `json:"stream"`
			Values [][]string        `json:"values"`
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
	if s.Stream["module"] != "rpc-server" {
		t.Errorf("module label: got %q (want rpc-server)", s.Stream["module"])
	}
}

func TestForwardLogs_SplitsByModule(t *testing.T) {
	// One payload with lines from multiple modules must produce one Loki stream
	// per (validator, level, module) tuple so `module` is an indexed label
	// (required for Grafana's label_values(module) dropdown).
	var lokiBody []byte
	lokiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lokiBody, _ = io.ReadAll(r.Body)
	}))
	defer lokiSrv.Close()

	payload := protocol.LogPayload{
		Level: "info",
		Lines: []json.RawMessage{
			json.RawMessage(`{"level":"info","ts":1.0,"msg":"a","module":"rpc-server"}`),
			json.RawMessage(`{"level":"info","ts":2.0,"msg":"b","module":"consensus"}`),
			json.RawMessage(`{"level":"info","ts":3.0,"msg":"c","module":"rpc-server"}`),
		},
	}
	body, _ := json.Marshal(payload)
	compressed, _ := zstdCompress(body)

	f := forwarder.New("http://vm-unused:8428", lokiSrv.URL)
	if err := f.ForwardLogs(context.Background(), "val-01", compressed, ""); err != nil {
		t.Fatalf("ForwardLogs: %v", err)
	}

	var push struct {
		Streams []struct {
			Stream map[string]string `json:"stream"`
			Values [][]string        `json:"values"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(lokiBody, &push); err != nil {
		t.Fatalf("parse loki body: %v", err)
	}
	byModule := make(map[string]int)
	for _, s := range push.Streams {
		if s.Stream["validator"] != "val-01" || s.Stream["level"] != "info" {
			t.Errorf("unexpected stream labels: %v", s.Stream)
		}
		byModule[s.Stream["module"]] += len(s.Values)
	}
	if byModule["rpc-server"] != 2 {
		t.Errorf("rpc-server line count: got %d, want 2", byModule["rpc-server"])
	}
	if byModule["consensus"] != 1 {
		t.Errorf("consensus line count: got %d, want 1", byModule["consensus"])
	}
	if len(byModule) != 2 {
		t.Errorf("distinct modules: got %d (%v), want 2", len(byModule), byModule)
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

	f := forwarder.New("http://vm-unused:8428", lokiSrv.URL)
	if err := f.ForwardLogs(context.Background(), "val-01", compressed, ""); err != nil {
		t.Fatalf("ForwardLogs: %v", err)
	}
	var push struct {
		Streams []struct {
			Stream map[string]string `json:"stream"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(lokiBody, &push); err != nil {
		t.Fatalf("parse loki body: %v", err)
	}
	if len(push.Streams) != 1 {
		t.Fatalf("stream count: got %d, want 1", len(push.Streams))
	}
	if push.Streams[0].Stream["module"] != "unknown" {
		t.Errorf("missing module: got %q, want 'unknown'", push.Streams[0].Stream["module"])
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

	f := forwarder.New(vmSrv.URL, "http://loki-unused:3100")
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
	f := forwarder.New("http://vm:8428", "http://loki:3100")
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

	f := forwarder.New(vmSrv.URL, "http://loki-unused:3100")
	err := f.ForwardRPC(context.Background(), "val-01", []byte(`{}`))
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
