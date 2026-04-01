// internal/sentinel/app/run_test.go
package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gnolang/val-companion/internal/sentinel/app"
	"github.com/gnolang/val-companion/internal/sentinel/config"
)

func TestRun_PostsRPCDataToServer(t *testing.T) {
	var received atomic.Int32

	// Mock watchtower.
	watchtower := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rpc" && r.Header.Get("Authorization") == "Bearer test-token" {
			received.Add(1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer watchtower.Close()

	// Mock gnoland node — height increments on each /status call only.
	var height atomic.Int64
	node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		type env struct {
			JSONRPC string `json:"jsonrpc"`
			Result  any    `json:"result"`
		}
		respond := func(result any) {
			b, _ := json.Marshal(env{JSONRPC: "2.0", Result: result})
			w.Write(b)
		}
		switch r.URL.Path {
		case "/status":
			h := height.Add(1)
			respond(map[string]any{
				"sync_info": map[string]any{
					"latest_block_height": fmt.Sprintf("%d", h),
					"catching_up":         false,
				},
			})
		default:
			respond(map[string]any{})
		}
	}))
	defer node.Close()

	cfg := &config.Config{
		Server: config.ServerConfig{URL: watchtower.URL, Token: "test-token"},
		RPC: config.RPCConfig{
			Enabled:                    true,
			PollInterval:               config.Duration{Duration: 50 * time.Millisecond},
			RPCURL:                     node.URL,
			DumpConsensusStateInterval: config.Duration{Duration: 1 * time.Hour},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()

	go app.Run(ctx, cfg)
	<-ctx.Done()

	if received.Load() == 0 {
		t.Error("expected at least one POST to /rpc, got none")
	}
}
