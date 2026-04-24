package doctor_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/sentinel/config"
	"github.com/aeddi/gno-watchtower/internal/sentinel/doctor"
	pkgnoise "github.com/aeddi/gno-watchtower/pkg/noise"
)

func TestCheckRemoteTokenAndPermissions_AllGreen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/check" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(doctor.AuthResponse{
			Validator:    "val-01",
			Permissions:  []string{"rpc", "metrics", "logs", "otlp"},
			LogsMinLevel: "info",
		})
	}))
	defer srv.Close()

	cfg := &config.Config{
		Server:    config.ServerConfig{URL: srv.URL, Token: "test-token"},
		RPC:       config.RPCConfig{Enabled: true},
		Logs:      config.LogsConfig{Enabled: true},
		OTLP:      config.OTLPConfig{Enabled: true},
		Resources: config.ResourcesConfig{Enabled: true},
		Metadata:  config.MetadataConfig{Enabled: true},
	}

	results, ar := doctor.CheckRemoteTokenAndPermissions(context.Background(), cfg)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Status != doctor.StatusGreen {
		t.Errorf("remote reachable: want GREEN, got %s: %s", results[0].Status, results[0].Detail)
	}
	if results[1].Status != doctor.StatusGreen {
		t.Errorf("token valid: want GREEN, got %s: %s", results[1].Status, results[1].Detail)
	}
	if results[2].Status != doctor.StatusGreen {
		t.Errorf("token permissions: want GREEN, got %s: %s", results[2].Status, results[2].Detail)
	}
	if ar == nil || ar.Validator != "val-01" {
		t.Errorf("expected AuthResponse with validator val-01, got %+v", ar)
	}
}

func TestCheckRemoteTokenAndPermissions_RemoteUnreachable(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{URL: "http://127.0.0.1:19999", Token: "tok"},
	}

	results, ar := doctor.CheckRemoteTokenAndPermissions(context.Background(), cfg)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Status != doctor.StatusRed {
		t.Errorf("want remote RED, got %s", results[0].Status)
	}
	if ar != nil {
		t.Error("expected nil AuthResponse when unreachable")
	}
}

func TestCheckRemoteTokenAndPermissions_TokenInvalid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	cfg := &config.Config{
		Server: config.ServerConfig{URL: srv.URL, Token: "bad-token"},
	}

	results, ar := doctor.CheckRemoteTokenAndPermissions(context.Background(), cfg)
	if results[0].Status != doctor.StatusGreen {
		t.Errorf("want remote GREEN (got a response), got %s", results[0].Status)
	}
	if results[1].Status != doctor.StatusRed {
		t.Errorf("want token RED, got %s", results[1].Status)
	}
	if ar != nil {
		t.Error("expected nil AuthResponse on 401")
	}
}

func TestCheckRemoteTokenAndPermissions_PermissionMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(doctor.AuthResponse{
			Validator:    "val-01",
			Permissions:  []string{"rpc"}, // missing metrics, logs, otlp
			LogsMinLevel: "info",
		})
	}))
	defer srv.Close()

	cfg := &config.Config{
		Server:    config.ServerConfig{URL: srv.URL, Token: "tok"},
		Logs:      config.LogsConfig{Enabled: true},
		Resources: config.ResourcesConfig{Enabled: true},
	}

	results, _ := doctor.CheckRemoteTokenAndPermissions(context.Background(), cfg)
	if results[2].Status != doctor.StatusRed {
		t.Errorf("want permissions RED, got %s: %s", results[2].Status, results[2].Detail)
	}
}

func TestCheckRemoteTokenAndPermissions_NoiseScheme_Unreachable(t *testing.T) {
	// A noise:// URL pointing at a dead address must fail with a transport
	// error (connection refused), not "unsupported protocol scheme noise://"
	// from the default http.Client. This proves the doctor wired up a Noise
	// transport for the noise:// scheme.
	cliKP, err := pkgnoise.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	keysDir := t.TempDir()
	if err := pkgnoise.WriteKeypair(keysDir, cliKP); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Server: config.ServerConfig{
			URL:   "noise://127.0.0.1:19999",
			Token: "tok",
		},
		Beacon: config.BeaconConfig{KeysDir: keysDir},
	}

	results, ar := doctor.CheckRemoteTokenAndPermissions(context.Background(), cfg)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Status != doctor.StatusRed {
		t.Errorf("want remote RED, got %s: %s", results[0].Status, results[0].Detail)
	}
	if strings.Contains(results[0].Detail, "unsupported protocol scheme") {
		t.Errorf("noise:// not wired through transport: %s", results[0].Detail)
	}
	if ar != nil {
		t.Error("expected nil AuthResponse when unreachable")
	}
}

func TestCheckRemoteTokenAndPermissions_NoiseScheme_Reachable(t *testing.T) {
	// Full end-to-end noise:// round trip: the doctor dials a Noise listener,
	// GETs /auth/check over the Noise-wrapped HTTP stream, and decodes the
	// response.
	srvKP, err := pkgnoise.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	cliKP, err := pkgnoise.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	lis, err := pkgnoise.NewListener(
		"tcp",
		"127.0.0.1:0",
		pkgnoise.Config{Static: srvKP},
		time.Second,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()

	httpSrv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/auth/check" || r.Header.Get("Authorization") != "Bearer tok" {
				http.Error(w, "bad", http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(doctor.AuthResponse{
				Validator:    "val-noise",
				Permissions:  []string{"rpc"},
				LogsMinLevel: "info",
			})
		}),
	}
	go httpSrv.Serve(lis) //nolint:errcheck
	defer httpSrv.Close()

	keysDir := t.TempDir()
	if err := pkgnoise.WriteKeypair(keysDir, cliKP); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Server: config.ServerConfig{
			URL:   "noise://" + lis.Addr().String(),
			Token: "tok",
		},
		Beacon: config.BeaconConfig{
			KeysDir:   keysDir,
			PublicKey: hex.EncodeToString(srvKP.Public),
		},
		RPC: config.RPCConfig{Enabled: true},
	}

	results, ar := doctor.CheckRemoteTokenAndPermissions(context.Background(), cfg)
	if results[0].Status != doctor.StatusGreen {
		t.Errorf("remote: want GREEN, got %s: %s", results[0].Status, results[0].Detail)
	}
	if ar == nil || ar.Validator != "val-noise" {
		t.Errorf("expected AuthResponse{Validator:val-noise}, got %+v", ar)
	}
}
