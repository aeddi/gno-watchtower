package augment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aeddi/gno-watchtower/internal/beacon/config"
	"github.com/aeddi/gno-watchtower/pkg/logger"
)

// newFakeGnoland returns an httptest.Server that impersonates a gnoland RPC:
// /status + /net_info wrapped in a JSON-RPC envelope, plus optional failure
// injection on specific paths.
func newFakeGnoland(t *testing.T, failPaths map[string]bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failPaths[r.URL.Path] {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/status":
			_, _ = w.Write([]byte(`{"result":{"node_info":{"moniker":"sentry-a","network":"gno-1"}}}`))
		case "/net_info":
			_, _ = w.Write([]byte(`{"result":{"n_peers":"42","peers":[]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

// withNetInfo wraps inner in a protocol.RPCPayload-like envelope:
// {"collected_at":"...", "data":{"net_info":<inner>}}.
func withNetInfo(inner string) []byte {
	return []byte(`{"collected_at":"2026-04-20T00:00:00Z","data":{"net_info":` + inner + `}}`)
}

func TestTransform_NonRPCPath_PassesThrough(t *testing.T) {
	a := New(&config.Config{RPC: config.RPCConfig{RPCURL: "http://localhost:26657"}}, nil, logger.Noop())
	out := a.Transform(context.Background(), "/logs", []byte("zstd-bytes"))
	if out != nil {
		t.Fatalf("non-/rpc path should pass through (nil), got %q", out)
	}
}

func TestTransform_RPCWithoutNetInfo_PassesThrough(t *testing.T) {
	srv := newFakeGnoland(t, nil)
	defer srv.Close()

	a := New(&config.Config{RPC: config.RPCConfig{RPCURL: srv.URL}}, nil, logger.Noop())
	payload := []byte(`{"data":{"block":{"height":"10"}}}`)
	if out := a.Transform(context.Background(), "/rpc", payload); out != nil {
		t.Fatalf("payload without net_info should pass through, got %q", out)
	}
}

func TestTransform_RPCWithoutDataField_PassesThrough(t *testing.T) {
	srv := newFakeGnoland(t, nil)
	defer srv.Close()

	a := New(&config.Config{RPC: config.RPCConfig{RPCURL: srv.URL}}, nil, logger.Noop())
	if out := a.Transform(context.Background(), "/rpc", []byte(`{"collected_at":"x"}`)); out != nil {
		t.Fatalf("payload without data should pass through, got %q", out)
	}
}

func TestTransform_RPCWithNetInfo_InjectsSentryKeys(t *testing.T) {
	srv := newFakeGnoland(t, nil)
	defer srv.Close()

	a := New(&config.Config{RPC: config.RPCConfig{RPCURL: srv.URL}}, nil, logger.Noop())
	body := withNetInfo(`{"n_peers":"7"}`)

	out := a.Transform(context.Background(), "/rpc", body)
	if out == nil {
		t.Fatal("expected augmented body, got nil")
	}

	var env struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	if _, ok := env.Data["net_info"]; !ok {
		t.Error("original net_info key missing from augmented data")
	}
	if _, ok := env.Data["sentry_status"]; !ok {
		t.Error("sentry_status not injected")
	}
	if _, ok := env.Data["sentry_net_info"]; !ok {
		t.Error("sentry_net_info not injected")
	}
	// sentry_config absent because metadata config is empty in this test.
	if _, ok := env.Data["sentry_config"]; ok {
		t.Error("sentry_config should be absent when metadata config is unset")
	}
}

// Per-key fail-open: one sentry hiccup costs only the failing key's series,
// not the whole augmentation. A missing /status still leaves sentry_net_info
// injected; a missing /net_info still leaves sentry_status injected.

func TestTransform_StatusFails_OtherKeysInjected(t *testing.T) {
	srv := newFakeGnoland(t, map[string]bool{"/status": true})
	defer srv.Close()

	a := New(&config.Config{RPC: config.RPCConfig{RPCURL: srv.URL}}, nil, logger.Noop())
	body := withNetInfo(`{"n_peers":"7"}`)
	out := a.Transform(context.Background(), "/rpc", body)
	if out == nil {
		t.Fatal("expected partial augmentation; got passthrough")
	}
	var env struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatal(err)
	}
	if _, ok := env.Data["sentry_status"]; ok {
		t.Error("sentry_status should be absent when /status fetch failed")
	}
	if _, ok := env.Data["sentry_net_info"]; !ok {
		t.Error("sentry_net_info should still be injected when only /status failed")
	}
}

func TestTransform_NetInfoFails_OtherKeysInjected(t *testing.T) {
	srv := newFakeGnoland(t, map[string]bool{"/net_info": true})
	defer srv.Close()

	a := New(&config.Config{RPC: config.RPCConfig{RPCURL: srv.URL}}, nil, logger.Noop())
	body := withNetInfo(`{"n_peers":"7"}`)
	out := a.Transform(context.Background(), "/rpc", body)
	if out == nil {
		t.Fatal("expected partial augmentation; got passthrough")
	}
	var env struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatal(err)
	}
	if _, ok := env.Data["sentry_net_info"]; ok {
		t.Error("sentry_net_info should be absent when /net_info fetch failed")
	}
	if _, ok := env.Data["sentry_status"]; !ok {
		t.Error("sentry_status should still be injected when only /net_info failed")
	}
}

func TestTransform_AllFetchesFail_PassesThrough(t *testing.T) {
	srv := newFakeGnoland(t, map[string]bool{"/status": true, "/net_info": true})
	defer srv.Close()

	a := New(&config.Config{RPC: config.RPCConfig{RPCURL: srv.URL}}, nil, logger.Noop())
	body := withNetInfo(`{"n_peers":"7"}`)
	if out := a.Transform(context.Background(), "/rpc", body); out != nil {
		t.Fatalf("all-failed augmentation should pass through; got %q", out)
	}
}

func TestTransform_ConfigPathMode_InjectsSentryConfig(t *testing.T) {
	srv := newFakeGnoland(t, nil)
	defer srv.Close()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(`[p2p]
pex = true
max_num_outbound_peers = 20
`), 0o644); err != nil {
		t.Fatal(err)
	}

	a := New(&config.Config{
		RPC:      config.RPCConfig{RPCURL: srv.URL},
		Metadata: config.MetadataConfig{ConfigPath: cfgPath},
	}, []string{"p2p.pex", "p2p.max_num_outbound_peers", "missing.key"}, logger.Noop())

	body := withNetInfo(`{"n_peers":"7"}`)
	out := a.Transform(context.Background(), "/rpc", body)
	if out == nil {
		t.Fatal("expected augmented body")
	}
	var env struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatal(err)
	}
	raw, ok := env.Data["sentry_config"]
	if !ok {
		t.Fatal("sentry_config not injected")
	}
	var got map[string]string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got["p2p.pex"] != "true" {
		t.Errorf("p2p.pex = %q, want \"true\"", got["p2p.pex"])
	}
	if got["p2p.max_num_outbound_peers"] != "20" {
		t.Errorf("p2p.max_num_outbound_peers = %q, want \"20\"", got["p2p.max_num_outbound_peers"])
	}
	if _, bad := got["missing.key"]; bad {
		t.Error("missing.key should have been skipped")
	}
}

func TestTransform_PreservesTopLevelFields(t *testing.T) {
	srv := newFakeGnoland(t, nil)
	defer srv.Close()

	a := New(&config.Config{RPC: config.RPCConfig{RPCURL: srv.URL}}, nil, logger.Noop())
	body := []byte(`{"collected_at":"2026-04-20T00:00:00Z","data":{"net_info":{"n_peers":"7"}},"extra":"keep"}`)

	out := a.Transform(context.Background(), "/rpc", body)
	if out == nil {
		t.Fatal("expected augmented body")
	}
	if !strings.Contains(string(out), `"extra":"keep"`) {
		t.Errorf("unknown top-level field 'extra' should be preserved, got %s", out)
	}
	if !strings.Contains(string(out), `"collected_at"`) {
		t.Errorf("collected_at should be preserved, got %s", out)
	}
}
