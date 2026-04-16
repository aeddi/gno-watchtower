package rpc_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/sentinel/rpc"
)

// rpcResponse mimics a gnoland JSON-RPC 2.0 envelope.
func rpcResponse(result any) []byte {
	type envelope struct {
		JSONRPC string `json:"jsonrpc"`
		ID      string `json:"id"`
		Result  any    `json:"result"`
	}
	b, err := json.Marshal(envelope{JSONRPC: "2.0", Result: result})
	if err != nil {
		panic("rpcResponse: marshal failed: " + err.Error())
	}
	return b
}

func TestClient_Status(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(rpcResponse(map[string]any{
			"sync_info": map[string]any{"latest_block_height": "100"},
		}))
	}))
	defer srv.Close()

	c := rpc.NewClient(srv.URL)
	raw, err := c.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("expected non-empty result")
	}

	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	syncInfo, ok := result["sync_info"].(map[string]any)
	if !ok {
		t.Fatal("expected sync_info in result")
	}
	if syncInfo["latest_block_height"] != "100" {
		t.Errorf("unexpected height: %v", syncInfo["latest_block_height"])
	}
}

func TestClient_RPCError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"jsonrpc":"2.0","error":{"code":-32600,"message":"invalid request"}}`))
	}))
	defer srv.Close()

	c := rpc.NewClient(srv.URL)
	_, err := c.Status(context.Background())
	if err == nil {
		t.Fatal("expected error from RPC error response")
	}
}

func TestClient_Block(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/block" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("height") != "42" {
			http.Error(w, "bad height", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(rpcResponse(map[string]any{"block_id": "abc"}))
	}))
	defer srv.Close()

	c := rpc.NewClient(srv.URL)
	raw, err := c.Block(context.Background(), 42)
	if err != nil {
		t.Fatalf("Block: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("expected non-empty result")
	}
}

func TestClient_Validators(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/validators" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("height") != "10" {
			http.Error(w, "bad height", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(rpcResponse(map[string]any{"validators": []any{}}))
	}))
	defer srv.Close()

	c := rpc.NewClient(srv.URL)
	raw, err := c.Validators(context.Background(), 10)
	if err != nil {
		t.Fatalf("Validators: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("expected non-empty result")
	}
}

func TestClient_BlockResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/block_results" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("height") != "7" {
			http.Error(w, "bad height", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(rpcResponse(map[string]any{"txs_results": nil}))
	}))
	defer srv.Close()

	c := rpc.NewClient(srv.URL)
	raw, err := c.BlockResults(context.Background(), 7)
	if err != nil {
		t.Fatalf("BlockResults: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("expected non-empty result")
	}
}

func TestClient_ContextCancellation(t *testing.T) {
	unblock := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-unblock
	}))
	defer srv.Close()
	defer close(unblock)

	ctx, cancel := context.WithCancel(context.Background())
	c := rpc.NewClient(srv.URL)

	done := make(chan error, 1)
	go func() {
		_, err := c.Status(ctx)
		done <- err
	}()

	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error after context cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Status did not return after context cancellation")
	}
}

func TestClient_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer srv.Close()

	c := rpc.NewClient(srv.URL)
	_, err := c.Status(context.Background())
	if err == nil {
		t.Fatal("expected error for non-200 HTTP status")
	}
}
