package doctor_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aeddi/gno-watchtower/internal/beacon/config"
	"github.com/aeddi/gno-watchtower/internal/beacon/doctor"
	pkgnoise "github.com/aeddi/gno-watchtower/pkg/noise"
)

func TestRun_AllGreen_ExitCode0(t *testing.T) {
	wt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer wt.Close()

	rpc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer rpc.Close()

	keysDir := t.TempDir()
	kp, err := pkgnoise.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	if err := pkgnoise.WriteKeypair(keysDir, kp); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Server: config.ServerConfig{URL: wt.URL},
		Beacon: config.BeaconConfig{KeysDir: keysDir},
		RPC:    config.RPCConfig{RPCURL: rpc.URL},
	}

	var buf bytes.Buffer
	code := doctor.Run(context.Background(), cfg, "test.toml", &buf)
	if code != 0 {
		t.Errorf("want exit 0, got %d\noutput:\n%s", code, buf.String())
	}
	out := buf.String()
	for _, want := range []string{"Watchtower", "Beacon keypair", "RPC"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing check %q in output:\n%s", want, out)
		}
	}
}

func TestRun_WatchtowerUnreachable_ExitCode1(t *testing.T) {
	keysDir := t.TempDir()
	kp, _ := pkgnoise.GenerateKeypair()
	pkgnoise.WriteKeypair(keysDir, kp) //nolint:errcheck

	cfg := &config.Config{
		Server: config.ServerConfig{URL: "http://127.0.0.1:19999"},
		Beacon: config.BeaconConfig{KeysDir: keysDir},
		RPC:    config.RPCConfig{RPCURL: "http://127.0.0.1:19998"},
	}

	var buf bytes.Buffer
	code := doctor.Run(context.Background(), cfg, "test.toml", &buf)
	if code != 1 {
		t.Errorf("want exit 1 when watchtower is down, got %d", code)
	}
}

func TestRun_AllOrangePlaceholders_ExitCode0(t *testing.T) {
	// Fresh generate-config produces placeholder values everywhere. Doctor on
	// that config should report Orange (not-configured) across the board and
	// still exit 0 — placeholders aren't a failure, they're "not configured
	// yet". This matches the sentinel doctor's treatment.
	cfg := &config.Config{
		Server:   config.ServerConfig{URL: "<watchtower-url>"},
		Beacon:   config.BeaconConfig{KeysDir: "<path-to-beacon-keys-dir>"},
		Metadata: config.MetadataConfig{ConfigPath: "<path-to-gnoland-config>"},
		// RPC.RPCURL intentionally unset (will still be Red, see below).
	}
	// Mute RPC Red by setting an OK server.
	rpc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer rpc.Close()
	cfg.RPC.RPCURL = rpc.URL

	var buf bytes.Buffer
	code := doctor.Run(context.Background(), cfg, "test.toml", &buf)
	if code != 0 {
		t.Errorf("want exit 0 for placeholder config, got %d\noutput:\n%s", code, buf.String())
	}
}

func TestRun_PrintsConfigPathHeader(t *testing.T) {
	cfg := &config.Config{
		Server:   config.ServerConfig{URL: "<watchtower-url>"},
		Beacon:   config.BeaconConfig{KeysDir: "<path-to-beacon-keys-dir>"},
		Metadata: config.MetadataConfig{ConfigPath: "<path-to-gnoland-config>"},
		RPC:      config.RPCConfig{RPCURL: "<rpc-url>"},
	}
	var buf bytes.Buffer
	doctor.Run(context.Background(), cfg, filepath.Join("some", "beacon.toml"), &buf)
	if !strings.Contains(buf.String(), "beacon.toml") {
		t.Errorf("run header did not include config path, output:\n%s", buf.String())
	}
}
