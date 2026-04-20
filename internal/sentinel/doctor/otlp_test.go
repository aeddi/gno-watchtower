package doctor_test

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/sentinel/doctor"
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
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addr+"/v1/metrics", bytes.NewReader(nil))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/x-protobuf")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return
		}
		resp.Body.Close()
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
