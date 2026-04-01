// internal/sentinel/otlp/relay_test.go
package otlp_test

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	collectorpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"

	"github.com/gnolang/val-companion/internal/sentinel/otlp"
	"github.com/gnolang/val-companion/pkg/logger"
)

func TestRelay_ForwardsExportToChannel(t *testing.T) {
	out := make(chan []byte, 1)

	// Find a free port.
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := lis.Addr().String()
	lis.Close()

	r := otlp.NewRelay(addr, out, logger.Noop())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		if err := r.Run(ctx); err != nil && ctx.Err() == nil {
			t.Errorf("relay.Run: %v", err)
		}
	}()

	// Give the server a moment to start.
	time.Sleep(50 * time.Millisecond)

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc dial: %v", err)
	}
	defer conn.Close()

	client := collectorpb.NewMetricsServiceClient(conn)
	req := &collectorpb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{},
	}
	if _, err := client.Export(ctx, req); err != nil {
		t.Fatalf("Export: %v", err)
	}

	select {
	case <-out:
		// payload arrived — relay successfully forwarded the export
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected bytes in channel within 500ms")
	}
}
