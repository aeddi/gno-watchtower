// internal/sentinel/doctor/otlp.go
package doctor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// otlpCheckDuration is how long CheckOTLP listens for an OTLP export.
const otlpCheckDuration = 3 * time.Second

// CheckOTLP starts an HTTP server on listenAddr and waits up to 3 seconds for
// gnoland to send an OTLP/HTTP metrics export to POST /v1/metrics. Mirrors the
// production relay at internal/sentinel/otlp/relay.go — gnoland posts protobuf
// over HTTP, not gRPC.
func CheckOTLP(ctx context.Context, listenAddr string) CheckResult {
	const name = "OTLP"

	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return CheckResult{Name: name, Status: StatusRed, Detail: fmt.Sprintf("listen %s: %v", listenAddr, err)}
	}

	received := make(chan struct{}, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/metrics", func(w http.ResponseWriter, _ *http.Request) {
		select {
		case received <- struct{}{}:
		default:
		}
		// Return a minimal 200 so gnoland's OTel SDK considers the push successful.
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
	})

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go srv.Serve(lis) //nolint:errcheck

	ctx, cancel := context.WithTimeout(ctx, otlpCheckDuration)
	defer cancel()
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
		defer shutdownCancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	select {
	case <-received:
		return CheckResult{Name: name, Status: StatusGreen, Detail: fmt.Sprintf("export received on %s", listenAddr)}
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return CheckResult{Name: name, Status: StatusRed, Detail: fmt.Sprintf("no export received in %s on %s", otlpCheckDuration, listenAddr)}
		}
		return CheckResult{Name: name, Status: StatusRed, Detail: fmt.Sprintf("check cancelled: %v", ctx.Err())}
	}
}
