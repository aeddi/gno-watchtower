package doctor_test

import (
	"context"
	"net"
	"testing"
	"time"

	collectorpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/gnolang/val-companion/internal/sentinel/doctor"
)

func TestCheckOTLP_Green_WhenExportReceived(t *testing.T) {
	// Find a free port.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := lis.Addr().String()
	lis.Close()

	// Send an export after a short delay so the check server is ready.
	go func() {
		time.Sleep(100 * time.Millisecond)
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return
		}
		defer conn.Close()
		client := collectorpb.NewMetricsServiceClient(conn)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		client.Export(ctx, &collectorpb.ExportMetricsServiceRequest{}) //nolint:errcheck
	}()

	r := doctor.CheckOTLP(context.Background(), addr)
	if r.Status != doctor.StatusGreen {
		t.Errorf("want GREEN, got %s: %s", r.Status, r.Detail)
	}
}

func TestCheckOTLP_Red_WhenNoExport(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := lis.Addr().String()
	lis.Close()

	// Nobody sends — check times out and returns Red.
	r := doctor.CheckOTLP(context.Background(), addr)
	if r.Status != doctor.StatusRed {
		t.Errorf("want RED, got %s: %s", r.Status, r.Detail)
	}
}
