// internal/sentinel/rpc/collector_test.go
package rpc_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/sentinel/rpc"
	"github.com/aeddi/gno-watchtower/pkg/logger"
	"github.com/aeddi/gno-watchtower/pkg/protocol"
)

// buildMockNode returns a test server that simulates a gnoland RPC node.
// height is incremented after each /status call to trigger new-block fetches.
func buildMockNode(t *testing.T) *httptest.Server {
	t.Helper()
	var height atomic.Int64
	height.Store(1)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		respond := func(result any) {
			type env struct {
				JSONRPC string `json:"jsonrpc"`
				Result  any    `json:"result"`
			}
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
		case "/net_info":
			respond(map[string]any{"n_peers": "3"})
		case "/num_unconfirmed_txs":
			respond(map[string]any{"n_txs": "0"})
		case "/dump_consensus_state":
			respond(map[string]any{"round_state": map[string]any{}})
		case "/validators":
			respond(map[string]any{"validators": []any{}})
		case "/block":
			respond(map[string]any{"block_id": "abc"})
		case "/genesis":
			respond(map[string]any{"genesis": map[string]any{"chain_id": "test-chain"}})
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestCollector_EmitsPayloads(t *testing.T) {
	srv := buildMockNode(t)
	defer srv.Close()

	out := make(chan protocol.RPCPayload, 10)
	c := rpc.NewCollector(rpc.NewClient(srv.URL), 50*time.Millisecond, 1*time.Hour, 0, 0, out, logger.Noop())

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	go c.Run(ctx)

	var received []protocol.RPCPayload
	deadline := time.After(300 * time.Millisecond)
loop:
	for {
		select {
		case p := <-out:
			received = append(received, p)
			if len(received) >= 2 {
				break loop
			}
		case <-deadline:
			break loop
		}
	}

	if len(received) < 2 {
		t.Fatalf("expected at least 2 payloads, got %d", len(received))
	}
	// First payload must contain status (always present on first poll).
	if _, ok := received[0].Data["status"]; !ok {
		t.Error("first payload missing 'status'")
	}
}

func TestCollector_GenesisRefreshInterval_ReEmitsGenesis(t *testing.T) {
	srv := buildMockNode(t)
	defer srv.Close()

	out := make(chan protocol.RPCPayload, 20)
	c := rpc.NewCollector(
		rpc.NewClient(srv.URL),
		20*time.Millisecond,
		1*time.Hour,
		50*time.Millisecond, // short genesis refresh for test
		1*time.Hour,         // validators refresh (not under test here)
		out,
		logger.Noop(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	go c.Run(ctx)

	var genesisCount int
	deadline := time.After(400 * time.Millisecond)
loop:
	for {
		select {
		case p := <-out:
			if _, ok := p.Data["genesis"]; ok {
				genesisCount++
				if genesisCount >= 2 {
					break loop
				}
			}
		case <-deadline:
			break loop
		}
	}
	if genesisCount < 2 {
		t.Fatalf("expected genesis to be re-emitted after refresh interval, got %d emissions", genesisCount)
	}
}

func TestCollector_ValidatorsRefreshInterval_ReEmitsValidators(t *testing.T) {
	srv := buildMockNode(t)
	defer srv.Close()

	out := make(chan protocol.RPCPayload, 20)
	c := rpc.NewCollector(
		rpc.NewClient(srv.URL),
		20*time.Millisecond,
		1*time.Hour,
		1*time.Hour,         // genesis refresh (not under test here)
		50*time.Millisecond, // short validators refresh for test
		out,
		logger.Noop(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	go c.Run(ctx)

	var validatorsCount int
	deadline := time.After(400 * time.Millisecond)
loop:
	for {
		select {
		case p := <-out:
			if _, ok := p.Data["validators"]; ok {
				validatorsCount++
				if validatorsCount >= 2 {
					break loop
				}
			}
		case <-deadline:
			break loop
		}
	}
	if validatorsCount < 2 {
		t.Fatalf("expected validators to be re-emitted after refresh interval, got %d emissions", validatorsCount)
	}
}

func TestCollector_DeltaSkipsUnchangedEndpoints(t *testing.T) {
	// Server always returns the same net_info response.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return static responses — status height never changes to avoid block fetches.
		type env struct {
			JSONRPC string `json:"jsonrpc"`
			Result  any    `json:"result"`
		}
		result := map[string]any{
			"sync_info":   map[string]any{"latest_block_height": "5", "catching_up": false},
			"n_peers":     "3",
			"n_txs":       "0",
			"round_state": map[string]any{},
		}
		b, _ := json.Marshal(env{JSONRPC: "2.0", Result: result})
		w.Write(b)
	}))
	defer srv.Close()

	out := make(chan protocol.RPCPayload, 10)
	c := rpc.NewCollector(rpc.NewClient(srv.URL), 50*time.Millisecond, 1*time.Hour, 0, 0, out, logger.Noop())

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go c.Run(ctx)
	<-ctx.Done()

	// Drain channel.
	var payloads []protocol.RPCPayload
	for len(out) > 0 {
		payloads = append(payloads, <-out)
	}

	// num_unconfirmed_txs bypasses the delta and is always included, so multiple
	// payloads are expected (one per poll tick).  Verify that subsequent payloads
	// contain only num_unconfirmed_txs and no other keys (delta still filters those).
	if len(payloads) < 1 {
		t.Fatal("expected at least one payload")
	}
	for i, p := range payloads[1:] {
		for key := range p.Data {
			if key != "num_unconfirmed_txs" {
				t.Errorf("payload[%d]: unexpected key %q — delta should have filtered it", i+1, key)
			}
		}
	}
}
