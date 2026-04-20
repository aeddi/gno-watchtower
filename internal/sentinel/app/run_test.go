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

	"github.com/aeddi/gno-watchtower/internal/sentinel/app"
	"github.com/aeddi/gno-watchtower/internal/sentinel/config"
	"github.com/aeddi/gno-watchtower/pkg/logger"
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

	go app.Run(ctx, cfg, logger.Noop())
	<-ctx.Done()

	if received.Load() == 0 {
		t.Error("expected at least one POST to /rpc, got none")
	}
}

func TestRun_RPCDisabled(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{URL: "http://localhost", Token: "x"},
		RPC:    config.RPCConfig{Enabled: false},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	// Must return without panic when RPC is disabled.
	app.Run(ctx, cfg, logger.Noop())
}

func TestRun_LogsEnabled_DockerUnavailable(t *testing.T) {
	// When Docker is unavailable, Run must start and exit cleanly without panicking.
	// The log collector logs its error and stops; RPC is not affected.
	cfg := &config.Config{
		Server: config.ServerConfig{URL: "http://localhost", Token: "tok"},
		Logs: config.LogsConfig{
			Enabled:       true,
			Source:        "docker",
			ContainerName: "nonexistent-container",
			BatchSize:     config.ByteSize(1024 * 1024),
			BatchTimeout:  config.Duration{Duration: 50 * time.Millisecond},
			MinLevel:      "info",
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	// Must return without panic when Docker is unavailable.
	app.Run(ctx, cfg, logger.Noop())
}

func TestRun_OTLPEnabled_ListenerFails(t *testing.T) {
	// When the OTLP listen address is valid (port 0 assigned by OS), Run starts and exits cleanly.
	cfg := &config.Config{
		Server: config.ServerConfig{URL: "http://localhost", Token: "tok"},
		OTLP: config.OTLPConfig{
			Enabled:    true,
			ListenAddr: "localhost:0", // port 0 is valid and will be assigned by OS
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	app.Run(ctx, cfg, logger.Noop())
}

func TestRun_ResourcesEnabled(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{URL: "http://localhost", Token: "tok"},
		Resources: config.ResourcesConfig{
			Enabled:      true,
			PollInterval: config.Duration{Duration: 10 * time.Millisecond},
			Source:       "host",
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	app.Run(ctx, cfg, logger.Noop())
}

func TestRun_MetadataEnabled(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{URL: "http://localhost", Token: "tok"},
		Metadata: config.MetadataConfig{
			Enabled:       true,
			CheckInterval: config.Duration{Duration: 10 * time.Millisecond},
			// No paths or commands set — no items to collect, exits cleanly.
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	app.Run(ctx, cfg, logger.Noop())
}

func TestRun_GracefulDrain(t *testing.T) {
	var receivedCount atomic.Int32

	watchtower := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rpc" {
			receivedCount.Add(1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer watchtower.Close()

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
				"sync_info": map[string]any{"latest_block_height": fmt.Sprintf("%d", h)},
			})
		default:
			respond(map[string]any{})
		}
	}))
	defer node.Close()

	cfg := &config.Config{
		Server: config.ServerConfig{URL: watchtower.URL, Token: "tok"},
		RPC: config.RPCConfig{
			Enabled:                    true,
			PollInterval:               config.Duration{Duration: 20 * time.Millisecond},
			RPCURL:                     node.URL,
			DumpConsensusStateInterval: config.Duration{Duration: 1 * time.Hour},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		app.Run(ctx, cfg, logger.Noop())
		close(done)
	}()

	// Let it collect a few payloads.
	time.Sleep(100 * time.Millisecond)
	cancel() // trigger shutdown

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}

	// After shutdown, all buffered items must have been sent.
	if receivedCount.Load() == 0 {
		t.Error("expected buffered payloads to be drained on shutdown")
	}
}
