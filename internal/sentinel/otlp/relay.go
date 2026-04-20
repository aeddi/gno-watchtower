// internal/sentinel/otlp/relay.go
package otlp

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	collectormetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracespb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

// Relay listens for OTLP/HTTP export requests from gnoland and forwards the
// raw metrics protobuf bytes to out for sending to watchtower POST /otlp.
//
// Only /v1/metrics is forwarded. /v1/traces is accepted with 200 OK and
// discarded — gnoland's OTel SDK needs a successful response to avoid retry
// storms when telemetry.traces_enabled is true, but we don't have a trace
// backend yet (see docs/data-collected.md "Known gaps").
//
// We use HTTP rather than gRPC because gnoland's traces/metrics init branches
// on the endpoint URL's scheme: http/https routes to the HTTP exporter (both
// metrics and traces), anything else routes to the gRPC metrics exporter and
// fails hard for traces ("unsupported scheme"). Running one protocol keeps
// the config surface simple and leaves the door open for adding real trace
// forwarding later without asking operators to reconfigure endpoints.
type Relay struct {
	listenAddr string
	out        chan<- []byte
	log        *slog.Logger
}

// NewRelay creates a Relay that listens on listenAddr and sends raw metrics
// protobuf bytes to out.
func NewRelay(listenAddr string, out chan<- []byte, log *slog.Logger) *Relay {
	return &Relay{
		listenAddr: listenAddr,
		out:        out,
		log:        log.With("component", "otlp_relay"),
	}
}

// Run starts the HTTP server and blocks until ctx is cancelled.
func (r *Relay) Run(ctx context.Context) error {
	lis, err := net.Listen("tcp", r.listenAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", r.listenAddr, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/metrics", r.handleMetrics)
	mux.HandleFunc("POST /v1/traces", r.handleTraces)
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		// Give inflight requests 5s to finish; then force-close.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	r.log.Info("listening", "addr", r.listenAddr)
	if err := srv.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve otlp: %w", err)
	}
	return nil
}

// handleMetrics parses an OTLP/HTTP ExportMetricsServiceRequest, applies the
// deny-list filter, re-marshals, and hands the bytes off to the sender loop
// via the non-blocking out channel. Gnoland's OTel SDK waits for an empty
// ExportMetricsServiceResponse protobuf to consider the push successful.
func (r *Relay) handleMetrics(w http.ResponseWriter, req *http.Request) {
	body, err := readBody(req)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	var msg collectormetricspb.ExportMetricsServiceRequest
	if err := proto.Unmarshal(body, &msg); err != nil {
		http.Error(w, "unmarshal metrics: "+err.Error(), http.StatusBadRequest)
		return
	}
	filterDenied(&msg)
	out, err := proto.Marshal(&msg)
	if err != nil {
		http.Error(w, "marshal metrics: "+err.Error(), http.StatusInternalServerError)
		return
	}
	select {
	case r.out <- out:
	default:
		r.log.Warn("otlp buffer full: payload dropped")
	}
	writeEmptyProtoResponse(w, &collectormetricspb.ExportMetricsServiceResponse{})
}

// handleTraces accepts and discards the trace export. The 200 OK keeps
// gnoland's OTel SDK happy (no retry loop) while we're not yet forwarding
// spans anywhere. See docs/data-collected.md for the roadmap.
func (r *Relay) handleTraces(w http.ResponseWriter, req *http.Request) {
	// Drain the body so the client sees a clean end-of-request.
	_, _ = io.Copy(io.Discard, req.Body)
	_ = req.Body.Close()
	writeEmptyProtoResponse(w, &collectortracespb.ExportTraceServiceResponse{})
}

// maxOTLPBodyBytes bounds how much we'll buffer from a single OTLP export. The
// listener is localhost-only but a misconfigured gnoland could still push a
// malformed or huge payload; cap at the same 50 MiB ceiling the watchtower
// uses for its /otlp endpoint (see watchtower/handlers maxBodyBytes).
const maxOTLPBodyBytes = 50 << 20

// readBody returns the request body, decompressing gzip if signalled by
// Content-Encoding. The OTel HTTP exporter opts into gzip by default.
func readBody(req *http.Request) ([]byte, error) {
	defer req.Body.Close()
	var reader io.Reader = io.LimitReader(req.Body, maxOTLPBodyBytes)
	if req.Header.Get("Content-Encoding") == "gzip" {
		gr, err := gzip.NewReader(reader)
		if err != nil {
			return nil, fmt.Errorf("gzip reader: %w", err)
		}
		defer gr.Close()
		reader = io.LimitReader(gr, maxOTLPBodyBytes)
	}
	return io.ReadAll(reader)
}

// writeEmptyProtoResponse serialises resp (an empty OTLP service response)
// and writes it with the 200-OK + application/x-protobuf headers the OTel
// spec requires. A marshal error on an empty generated message is a bug, not
// a runtime condition — fall through to a 500 so the caller notices.
func writeEmptyProtoResponse(w http.ResponseWriter, resp proto.Message) {
	b, err := proto.Marshal(resp)
	if err != nil {
		http.Error(w, "marshal response: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-protobuf")
	_, _ = w.Write(b)
}
