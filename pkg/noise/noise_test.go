package noise_test

import (
	"context"
	"crypto/rand"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/pkg/noise"
)

// mustKey generates a fresh keypair or fails the test.
func mustKey(t *testing.T) noise.Keypair {
	t.Helper()
	kp, err := noise.GenerateKeypair()
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	return kp
}

// listenerAddr starts a Noise listener on an ephemeral port and returns its
// address plus a cleanup.
func listenerAddr(t *testing.T, cfg noise.Config) (string, *noise.Listen) {
	t.Helper()
	lis, err := noise.NewListener("tcp", "127.0.0.1:0", cfg, time.Second, nil)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { lis.Close() })
	return lis.Addr().String(), lis
}

// runRoundTrip dials, sends payload, echoes it back on the server side, and
// returns what the client received. Covers end-to-end encrypt/decrypt.
//
// The server goroutine is fire-and-forget: if the handshake or authorization
// rejects the connection, the listener loop absorbs the error and the server
// goroutine stays parked in Accept until t.Cleanup closes the listener at
// test teardown. That's fine for tests — nothing leaks beyond the process.
func runRoundTrip(t *testing.T, cliCfg, srvCfg noise.Config, payload []byte) ([]byte, error) {
	t.Helper()

	addr, lis := listenerAddr(t, srvCfg)

	go func() {
		conn, err := lis.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, len(payload))
		if _, err := io.ReadFull(conn, buf); err != nil {
			return
		}
		_, _ = conn.Write(buf)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cconn, err := noise.Dial(ctx, "tcp", addr, cliCfg)
	if err != nil {
		return nil, err
	}
	defer cconn.Close()

	if _, err := cconn.Write(payload); err != nil {
		return nil, err
	}
	_ = cconn.SetReadDeadline(time.Now().Add(2 * time.Second))
	out := make([]byte, len(payload))
	if _, err := io.ReadFull(cconn, out); err != nil {
		return nil, err
	}
	return out, nil
}

// Mode 1: both sides anonymous (confidentiality only).
func TestNoise_Mode1_BothAnon_RoundTrip(t *testing.T) {
	cli := mustKey(t)
	srv := mustKey(t)
	out, err := runRoundTrip(t,
		noise.Config{Static: cli},
		noise.Config{Static: srv},
		[]byte("hello world"),
	)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	if string(out) != "hello world" {
		t.Errorf("echo got %q", out)
	}
}

// Mode 2: sentinel (initiator) verifies beacon (responder) pubkey.
// Currently modeled as initiator-side verification by passing the responder's
// expected pubkey in the initiator's AuthorizedKeys. Success path.
func TestNoise_Mode2_InitiatorVerifiesResponder_Success(t *testing.T) {
	cli := mustKey(t)
	srv := mustKey(t)
	out, err := runRoundTrip(t,
		noise.Config{Static: cli, AuthorizedKeys: [][]byte{srv.Public}},
		noise.Config{Static: srv},
		[]byte("mode2"),
	)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	if string(out) != "mode2" {
		t.Errorf("echo got %q", out)
	}
}

// Mode 2 failure: initiator expects a different responder pubkey.
func TestNoise_Mode2_InitiatorVerifiesResponder_RejectsWrongKey(t *testing.T) {
	cli := mustKey(t)
	srv := mustKey(t)
	wrong := mustKey(t).Public
	_, err := runRoundTrip(t,
		noise.Config{Static: cli, AuthorizedKeys: [][]byte{wrong}},
		noise.Config{Static: srv},
		[]byte("never arrives"),
	)
	if err == nil {
		t.Fatal("expected error rejecting wrong responder pubkey")
	}
	if !strings.Contains(err.Error(), "not in authorized_keys") {
		t.Errorf("expected authorization error, got: %v", err)
	}
}

// Mode 3: beacon (responder) verifies sentinel (initiator) pubkey. Success.
func TestNoise_Mode3_ResponderVerifiesInitiator_Success(t *testing.T) {
	cli := mustKey(t)
	srv := mustKey(t)
	out, err := runRoundTrip(t,
		noise.Config{Static: cli},
		noise.Config{Static: srv, AuthorizedKeys: [][]byte{cli.Public}},
		[]byte("mode3"),
	)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	if string(out) != "mode3" {
		t.Errorf("echo got %q", out)
	}
}

// Mode 3 failure: responder's whitelist doesn't include initiator's pubkey.
func TestNoise_Mode3_ResponderVerifiesInitiator_RejectsWrongKey(t *testing.T) {
	cli := mustKey(t)
	srv := mustKey(t)
	other := mustKey(t).Public
	_, err := runRoundTrip(t,
		noise.Config{Static: cli},
		noise.Config{Static: srv, AuthorizedKeys: [][]byte{other}},
		[]byte("nope"),
	)
	if err == nil {
		t.Fatal("expected error from responder rejecting unknown initiator")
	}
	// The initiator sees the connection close / framing error when the
	// responder rejects post-handshake. The specific message may vary; what
	// matters is that no round-trip succeeds.
}

// Mode 4: mutual authentication. Both sides have the other's pubkey in
// AuthorizedKeys; handshake and round-trip succeed.
func TestNoise_Mode4_MutualAuth_Success(t *testing.T) {
	cli := mustKey(t)
	srv := mustKey(t)
	out, err := runRoundTrip(t,
		noise.Config{Static: cli, AuthorizedKeys: [][]byte{srv.Public}},
		noise.Config{Static: srv, AuthorizedKeys: [][]byte{cli.Public}},
		[]byte("mutual"),
	)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	if string(out) != "mutual" {
		t.Errorf("echo got %q", out)
	}
}

// Large payloads span multiple Noise frames via the chunking Write path.
func TestNoise_LargePayload_Chunked(t *testing.T) {
	cli := mustKey(t)
	srv := mustKey(t)

	payload := make([]byte, 250_000) // roughly 4 frames at 64KB each
	if _, err := io.ReadFull(rand.Reader, payload); err != nil {
		t.Fatal(err)
	}

	out, err := runRoundTrip(t,
		noise.Config{Static: cli},
		noise.Config{Static: srv},
		payload,
	)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	if len(out) != len(payload) {
		t.Fatalf("echo length: got %d, want %d", len(out), len(payload))
	}
	for i := range payload {
		if payload[i] != out[i] {
			t.Fatalf("byte mismatch at offset %d", i)
		}
	}
}

// PeerStatic returns the correct peer key on both sides after a handshake.
func TestNoise_PeerStatic_ReflectsActualKey(t *testing.T) {
	cli := mustKey(t)
	srv := mustKey(t)

	addr, lis := listenerAddr(t, noise.Config{Static: srv})

	sconnCh := make(chan net.Conn, 1)
	go func() {
		c, err := lis.Accept()
		if err != nil {
			return
		}
		sconnCh <- c
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cconn, err := noise.Dial(ctx, "tcp", addr, noise.Config{Static: cli})
	if err != nil {
		t.Fatal(err)
	}
	defer cconn.Close()

	select {
	case sconn := <-sconnCh:
		defer sconn.Close()
		serverSide := sconn.(*noise.Conn).PeerStatic()
		if !bytesEq(serverSide, cli.Public) {
			t.Errorf("server PeerStatic: got %x, want %x", serverSide, cli.Public)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server never accepted")
	}
	clientSide := cconn.PeerStatic()
	if !bytesEq(clientSide, srv.Public) {
		t.Errorf("client PeerStatic: got %x, want %x", clientSide, srv.Public)
	}
}

func bytesEq(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
