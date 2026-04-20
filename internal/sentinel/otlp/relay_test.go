// internal/sentinel/otlp/relay_test.go
package otlp_test

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	collectormetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracespb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/proto"

	"github.com/aeddi/gno-watchtower/internal/sentinel/otlp"
	"github.com/aeddi/gno-watchtower/pkg/logger"
)

// startRelay starts a Relay on a free port, returns its base URL, and
// registers a t.Cleanup hook that shuts it down and waits for Run to return.
func startRelay(t *testing.T, out chan []byte) string {
	t.Helper()
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := lis.Addr().String()
	lis.Close()

	ctx, cancel := context.WithCancel(context.Background())
	r := otlp.NewRelay(addr, out, logger.Noop())
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := r.Run(ctx); err != nil && ctx.Err() == nil {
			t.Errorf("relay.Run: %v", err)
		}
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	// Poll until the listener accepts an HTTP connection. Probe a bogus path
	// so the warmup doesn't push an empty metrics payload onto the out
	// channel (which would pollute subsequent assertions).
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + addr + "/health")
		if err == nil {
			resp.Body.Close()
			return "http://" + addr
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("relay never started on %s", addr)
	return ""
}

func TestRelay_MetricsPost_ForwardsToChannel(t *testing.T) {
	out := make(chan []byte, 1)
	base := startRelay(t, out)

	req := &collectormetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{}},
	}
	body, err := proto.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	resp, err := http.Post(base+"/v1/metrics", "application/x-protobuf", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/metrics: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	select {
	case <-out:
		// Bytes arrived — relay forwarded successfully.
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected bytes on out channel within 500ms")
	}
}

func TestRelay_TracesPost_AcceptedButNotForwarded(t *testing.T) {
	out := make(chan []byte, 1)
	base := startRelay(t, out)

	req := &collectortracespb.ExportTraceServiceRequest{}
	body, err := proto.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	resp, err := http.Post(base+"/v1/traces", "application/x-protobuf", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/traces: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	select {
	case b := <-out:
		t.Fatalf("traces must not reach the forwarder out channel; got %d bytes", len(b))
	case <-time.After(100 * time.Millisecond):
		// Nothing forwarded — correct behavior for the no-backend state.
	}
}

func TestRelay_MetricsPost_MalformedBody_400(t *testing.T) {
	out := make(chan []byte, 1)
	base := startRelay(t, out)

	resp, err := http.Post(base+"/v1/metrics", "application/x-protobuf", bytes.NewReader([]byte("not-protobuf")))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestRelay_WrongMethod_MethodNotAllowed(t *testing.T) {
	out := make(chan []byte, 1)
	base := startRelay(t, out)

	resp, err := http.Get(base + "/v1/metrics")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	// stdlib mux returns 405 for method mismatch on the pattern "POST /v1/metrics".
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", resp.StatusCode)
	}
}
