package doctor_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gnolang/val-companion/internal/sentinel/config"
	"github.com/gnolang/val-companion/internal/sentinel/doctor"
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
