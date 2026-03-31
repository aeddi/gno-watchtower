// internal/sentinel/sender/sender_test.go
package sender_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gnolang/val-companion/internal/sentinel/sender"
)

func TestSender_SendSuccess(t *testing.T) {
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var readErr error
		received, readErr = io.ReadAll(r.Body)
		if readErr != nil {
			t.Errorf("read body: %v", readErr)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := sender.New(srv.URL, "test-token")
	payload := map[string]string{"hello": "world"}
	err := s.Send(context.Background(), "/rpc", payload)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	var got map[string]string
	if err := json.Unmarshal(received, &got); err != nil {
		t.Fatalf("unmarshal received body: %v", err)
	}
	if got["hello"] != "world" {
		t.Errorf("payload: got %v", got)
	}
}

func TestSender_SendHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	s := sender.New(srv.URL, "token")
	err := s.Send(context.Background(), "/rpc", map[string]string{})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestSender_SendWithRetry_RetriesOnFailure(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := sender.New(srv.URL, "token")
	err := s.SendWithRetry(context.Background(), "/rpc", map[string]string{}, 5, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("SendWithRetry: %v", err)
	}
	if attempts.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts.Load())
	}
}

func TestSender_SendWithRetry_ExhaustsAttempts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "always fail", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	s := sender.New(srv.URL, "token")
	err := s.SendWithRetry(context.Background(), "/rpc", map[string]string{}, 3, 5*time.Millisecond)
	if err == nil {
		t.Fatal("expected error after exhausting attempts")
	}
}

func TestSender_SendWithRetry_RespectsContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fail", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	s := sender.New(srv.URL, "token")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := s.SendWithRetry(ctx, "/rpc", map[string]string{}, 10, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected error when context cancelled")
	}
}
