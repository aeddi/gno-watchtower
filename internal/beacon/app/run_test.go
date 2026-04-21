package app_test

import (
	"context"
	"encoding/hex"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/beacon/app"
	"github.com/aeddi/gno-watchtower/internal/beacon/config"
	"github.com/aeddi/gno-watchtower/pkg/logger"
	"github.com/aeddi/gno-watchtower/pkg/noise"
	"github.com/aeddi/gno-watchtower/pkg/tomlutil"
)

// freePort returns an unused high-numbered TCP port.
func freePort(t *testing.T) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := lis.Addr().(*net.TCPAddr)
	lis.Close()
	return addr.String()
}

// writeKeys generates a Noise keypair under dir and returns the keypair.
func writeKeys(t *testing.T, dir string) noise.Keypair {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	kp, err := noise.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	if err := noise.WriteKeypair(dir, kp); err != nil {
		t.Fatal(err)
	}
	return kp
}

// TestRun_ServesAndShutdowns verifies that Run wires the server up, serves a
// request from a Noise client, and exits cleanly when the context is cancelled.
func TestRun_ServesAndShutdowns(t *testing.T) {
	// Fake upstream watchtower.
	upstreamHit := make(chan string, 1)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHit <- r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer up.Close()

	keysDir := filepath.Join(t.TempDir(), "keys")
	srvKP := writeKeys(t, keysDir)

	cliKP, err := noise.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}

	addr := freePort(t)
	cfg := &config.Config{
		Server: config.ServerConfig{URL: up.URL},
		Beacon: config.BeaconConfig{
			ListenAddr:       addr,
			KeysDir:          keysDir,
			AuthorizedKeys:   []string{hex.EncodeToString(cliKP.Public)},
			HandshakeTimeout: tomlutil.Duration{Duration: time.Second},
		},
		RPC: config.RPCConfig{RPCURL: "http://localhost:26657"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		app.Run(ctx, cfg, logger.Noop())
	}()

	// Wait for the listener to bind.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			c.Close()
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(c context.Context, network, a string) (net.Conn, error) {
				return noise.Dial(c, network, a, noise.Config{
					Static:         cliKP,
					AuthorizedKeys: [][]byte{srvKP.Public},
				})
			},
		},
		Timeout: 3 * time.Second,
	}
	resp, err := client.Get("http://" + addr + "/health")
	if err != nil {
		t.Fatalf("noise client: %v", err)
	}
	resp.Body.Close()

	select {
	case path := <-upstreamHit:
		if path != "/health" {
			t.Errorf("upstream got path %q, want /health", path)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("upstream never got the forwarded request")
	}

	cancel()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return within 3s of ctx cancellation")
	}
}
