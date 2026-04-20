// internal/sentinel/otlp/relay.go
package otlp

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	collectorpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// Relay listens for OTLP gRPC ExportMetricsServiceRequest calls from gnoland and
// forwards the raw protobuf bytes to out for sending to watchtower POST /otlp.
type Relay struct {
	listenAddr string
	out        chan<- []byte
	log        *slog.Logger
}

// NewRelay creates a Relay that listens on listenAddr and sends raw protobuf bytes to out.
func NewRelay(listenAddr string, out chan<- []byte, log *slog.Logger) *Relay {
	return &Relay{
		listenAddr: listenAddr,
		out:        out,
		log:        log.With("component", "otlp_relay"),
	}
}

// Run starts the gRPC server and blocks until ctx is cancelled.
func (r *Relay) Run(ctx context.Context) error {
	lis, err := net.Listen("tcp", r.listenAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", r.listenAddr, err)
	}

	srv := grpc.NewServer()
	collectorpb.RegisterMetricsServiceServer(srv, &metricsServer{out: r.out, log: r.log})

	go func() {
		<-ctx.Done()
		srv.GracefulStop()
	}()

	r.log.Info("listening", "addr", r.listenAddr)
	if err := srv.Serve(lis); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("serve otlp: %w", err)
	}
	return nil
}

// metricsServer implements the OTLP MetricsService gRPC interface.
type metricsServer struct {
	collectorpb.UnimplementedMetricsServiceServer
	out chan<- []byte
	log *slog.Logger
}

// Export receives an OTLP export request, drops metrics that are better
// served by the RPC collector (see deniedMetricNames), marshals the remainder,
// and sends to out (non-blocking).
func (m *metricsServer) Export(_ context.Context, req *collectorpb.ExportMetricsServiceRequest) (*collectorpb.ExportMetricsServiceResponse, error) {
	filterDenied(req)
	b, err := proto.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal otlp request: %w", err)
	}
	select {
	case m.out <- b:
	default:
		m.log.Warn("otlp buffer full: payload dropped")
	}
	return &collectorpb.ExportMetricsServiceResponse{}, nil
}
