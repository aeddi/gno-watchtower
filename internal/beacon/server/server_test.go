package server_test

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/beacon/server"
	"github.com/aeddi/gno-watchtower/pkg/logger"
	"github.com/aeddi/gno-watchtower/pkg/noise"
)

// freePort returns an unused high-numbered TCP port. Tests use a known port
// because the server currently binds via ListenAddr (no Addr() exposed).
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

// waitForDial blocks until a TCP Dial to addr succeeds.
func waitForDial(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			c.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("listener never bound %s", addr)
}

// newNoiseClient returns an http.Client whose Transport dials the beacon with
// pkg/noise. The beacon's upstream is the caller's responsibility.
func newNoiseClient(t *testing.T, cliKP, srvPub noise.Keypair) *http.Client {
	t.Helper()
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(c context.Context, network, addr string) (net.Conn, error) {
				return noise.Dial(c, network, addr, noise.Config{
					Static:         cliKP,
					AuthorizedKeys: [][]byte{srvPub.Public},
				})
			},
		},
		Timeout: 5 * time.Second,
	}
}

func TestBeaconServer_PassesThroughHeadersAndBody(t *testing.T) {
	gotBody := make(chan []byte, 1)
	gotAuth := make(chan string, 1)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody <- b
		gotAuth <- r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer up.Close()

	srvKP, _ := noise.GenerateKeypair()
	cliKP, _ := noise.GenerateKeypair()
	srvNoise := noise.Config{Static: srvKP}
	addr := freePort(t)

	s, err := server.New(server.Config{
		ListenAddr:       addr,
		UpstreamURL:      up.URL,
		NoiseConfig:      &srvNoise,
		HandshakeTimeout: time.Second,
		Log:              logger.Noop(),
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx) //nolint:errcheck
	waitForDial(t, addr, 2*time.Second)

	client := newNoiseClient(t, cliKP, srvKP)
	req, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/rpc", bytes.NewReader([]byte(`hello-body`)))
	req.Header.Set("Authorization", "Bearer my-secret")
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d", resp.StatusCode)
	}

	select {
	case b := <-gotBody:
		if !bytes.Equal(b, []byte(`hello-body`)) {
			t.Errorf("body mismatch: got %q", b)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("upstream never received body")
	}
	select {
	case a := <-gotAuth:
		if a != "Bearer my-secret" {
			t.Errorf("auth header mismatch: got %q", a)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("upstream never received auth header")
	}
}

func TestBeaconServer_TransformRewritesBody(t *testing.T) {
	gotBody := make(chan []byte, 1)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody <- b
		w.WriteHeader(http.StatusOK)
	}))
	defer up.Close()

	srvKP, _ := noise.GenerateKeypair()
	cliKP, _ := noise.GenerateKeypair()
	srvNoise := noise.Config{Static: srvKP}
	addr := freePort(t)

	transform := func(ctx context.Context, path string, body []byte) []byte {
		if path != "/rpc" {
			return nil
		}
		return append([]byte("AUGMENTED:"), body...)
	}

	s, err := server.New(server.Config{
		ListenAddr:       addr,
		UpstreamURL:      up.URL,
		NoiseConfig:      &srvNoise,
		HandshakeTimeout: time.Second,
		Transform:        transform,
		Log:              logger.Noop(),
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx) //nolint:errcheck
	waitForDial(t, addr, 2*time.Second)

	client := newNoiseClient(t, cliKP, srvKP)
	req, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/rpc", bytes.NewReader([]byte(`original`)))
	resp, _ := client.Do(req)
	if resp != nil {
		resp.Body.Close()
	}

	select {
	case b := <-gotBody:
		if !strings.HasPrefix(string(b), "AUGMENTED:") {
			t.Errorf("body not augmented: got %q", b)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("upstream never received body")
	}
}

func TestBeaconServer_TransformOnOtherPathsPassesThrough(t *testing.T) {
	gotBody := make(chan []byte, 1)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody <- b
		w.WriteHeader(http.StatusOK)
	}))
	defer up.Close()

	srvKP, _ := noise.GenerateKeypair()
	cliKP, _ := noise.GenerateKeypair()
	srvNoise := noise.Config{Static: srvKP}
	addr := freePort(t)

	transformCalls := 0
	transform := func(ctx context.Context, path string, body []byte) []byte {
		transformCalls++
		if path != "/rpc" {
			return nil
		}
		return append([]byte("AUGMENTED:"), body...)
	}

	s, err := server.New(server.Config{
		ListenAddr:       addr,
		UpstreamURL:      up.URL,
		NoiseConfig:      &srvNoise,
		HandshakeTimeout: time.Second,
		Transform:        transform,
		Log:              logger.Noop(),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx) //nolint:errcheck
	waitForDial(t, addr, 2*time.Second)

	client := newNoiseClient(t, cliKP, srvKP)
	req, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/logs", bytes.NewReader([]byte(`zstd-bytes`)))
	_, _ = client.Do(req)

	select {
	case b := <-gotBody:
		if !bytes.Equal(b, []byte(`zstd-bytes`)) {
			t.Errorf("body should be untouched: got %q", b)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("upstream never received body")
	}
	if transformCalls != 1 {
		t.Errorf("transform call count: got %d, want 1", transformCalls)
	}
}
