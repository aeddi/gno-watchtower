// internal/sentinel/doctor/otlp.go
package doctor

import (
	"context"
	"fmt"
	"net"
	"time"

	collectorpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc"
)

// otlpCheckDuration is how long CheckOTLP listens for an OTLP export.
const otlpCheckDuration = 3 * time.Second

// CheckOTLP starts a gRPC server on listenAddr and waits up to 3 seconds for gnoland to send
// an OTLP metrics export. Green = received at least one export; Red = none received or port conflict.
func CheckOTLP(ctx context.Context, listenAddr string) CheckResult {
	const name = "OTLP"

	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return CheckResult{Name: name, Status: StatusRed, Detail: fmt.Sprintf("listen %s: %v", listenAddr, err)}
	}

	received := make(chan struct{}, 1)
	srv := grpc.NewServer()
	collectorpb.RegisterMetricsServiceServer(srv, &otlpCheckServer{received: received})

	go srv.Serve(lis) //nolint:errcheck

	ctx, cancel := context.WithTimeout(ctx, otlpCheckDuration)
	defer cancel()
	defer srv.GracefulStop()

	select {
	case <-received:
		return CheckResult{Name: name, Status: StatusGreen, Detail: fmt.Sprintf("export received on %s", listenAddr)}
	case <-ctx.Done():
		return CheckResult{Name: name, Status: StatusRed, Detail: fmt.Sprintf("no export received in %s on %s", otlpCheckDuration, listenAddr)}
	}
}

// otlpCheckServer signals received on the first Export call.
type otlpCheckServer struct {
	collectorpb.UnimplementedMetricsServiceServer
	received chan struct{}
}

func (s *otlpCheckServer) Export(_ context.Context, _ *collectorpb.ExportMetricsServiceRequest) (*collectorpb.ExportMetricsServiceResponse, error) {
	select {
	case s.received <- struct{}{}:
	default:
	}
	return &collectorpb.ExportMetricsServiceResponse{}, nil
}
