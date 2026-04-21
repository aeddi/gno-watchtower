// internal/sentinel/sender/sender_test.go
package sender_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/sentinel/sender"
	pkgnoise "github.com/aeddi/gno-watchtower/pkg/noise"
)

func mustNew(t *testing.T, url, token string) *sender.Sender {
	t.Helper()
	s, err := sender.New(url, token, nil)
	if err != nil {
		t.Fatalf("sender.New: %v", err)
	}
	return s
}

func TestSender_SendRaw_SetsContentType(t *testing.T) {
	var gotContentType string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := mustNew(t, srv.URL, "tok")
	body := []byte{0x0a, 0x05, 0x68, 0x65, 0x6c, 0x6c, 0x6f} // arbitrary bytes
	if err := s.SendRaw(context.Background(), "/otlp", body, "application/x-protobuf"); err != nil {
		t.Fatalf("SendRaw: %v", err)
	}
	if gotContentType != "application/x-protobuf" {
		t.Errorf("Content-Type: got %q, want %q", gotContentType, "application/x-protobuf")
	}
	if string(gotBody) != string(body) {
		t.Errorf("body mismatch: got %x, want %x", gotBody, body)
	}
}

func TestSender_SendRaw_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := mustNew(t, srv.URL, "wrong-token")
	err := s.SendRaw(context.Background(), "/rpc", []byte("{}"), "application/json")
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
}

func TestSender_SendRawWithRetry_RetriesOnFailure(t *testing.T) {
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

	s := mustNew(t, srv.URL, "tok")
	err := s.SendRawWithRetry(context.Background(), "/otlp", []byte("data"), "application/x-protobuf", 5, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("SendRawWithRetry: %v", err)
	}
	if attempts.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts.Load())
	}
}

func TestSender_SendRawWithRetry_ExhaustsAttempts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "always fail", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	s := mustNew(t, srv.URL, "tok")
	err := s.SendRawWithRetry(context.Background(), "/rpc", []byte("{}"), "application/json", 3, 5*time.Millisecond)
	if err == nil {
		t.Fatal("expected error after exhausting attempts")
	}
}

func TestSender_SendRawWithRetry_RespectsContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fail", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	s := mustNew(t, srv.URL, "tok")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := s.SendRawWithRetry(ctx, "/rpc", []byte("{}"), "application/json", 10, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected error when context cancelled")
	}
}

func TestSender_SendRawWithRetry_RespectsRetryAfter(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n < 2 {
			w.Header().Set("Retry-After", "0") // 0 seconds: skip the normal backoff
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Use a large initialBackoff so that normal backoff would take ~500ms.
	// With Retry-After: 0, the retry must happen without waiting.
	s := mustNew(t, srv.URL, "tok")
	start := time.Now()
	err := s.SendRawWithRetry(context.Background(), "/rpc", []byte("{}"), "application/json", 3, 500*time.Millisecond)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("SendRawWithRetry: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("expected 2 calls, got %d", calls.Load())
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("expected Retry-After: 0 to skip backoff, elapsed %v", elapsed)
	}
}

func TestSender_SendCompressedBytesWithRetry_SetsHeaderAndBody(t *testing.T) {
	var gotEncoding string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotEncoding = r.Header.Get("Content-Encoding")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	payload := map[string]string{"key": "value"}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	compressed := sender.ZstdCompress(b)

	s := mustNew(t, srv.URL, "tok")
	if err := s.SendCompressedBytesWithRetry(context.Background(), "/logs", compressed, 1, 0); err != nil {
		t.Fatalf("SendCompressedBytesWithRetry: %v", err)
	}
	if gotEncoding != "zstd" {
		t.Errorf("Content-Encoding: got %q, want %q", gotEncoding, "zstd")
	}
	if !bytes.Equal(gotBody, compressed) {
		t.Errorf("body mismatch: got %x, want %x", gotBody, compressed)
	}
}

func TestSender_NoiseScheme_RoundTrip(t *testing.T) {
	// Responder side: Noise listener + http.Server that records the body.
	srvKP, err := pkgnoise.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	cliKP, err := pkgnoise.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	lis, err := pkgnoise.NewListener("tcp", "127.0.0.1:0", pkgnoise.Config{Static: srvKP}, time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()

	var gotToken string
	var gotBody []byte
	httpSrv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotToken = r.Header.Get("Authorization")
			gotBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
		}),
	}
	go httpSrv.Serve(lis) //nolint:errcheck
	defer httpSrv.Close()

	// Initiator: sentinel sender with noise:// scheme.
	url := "noise://" + lis.Addr().String()
	s, err := sender.New(url, "my-token", &pkgnoise.Config{Static: cliKP, AuthorizedKeys: [][]byte{srvKP.Public}})
	if err != nil {
		t.Fatalf("sender.New: %v", err)
	}
	body := []byte(`{"hello":"world"}`)
	if err := s.SendRaw(context.Background(), "/rpc", body, "application/json"); err != nil {
		t.Fatalf("SendRaw via noise://: %v", err)
	}
	if gotToken != "Bearer my-token" {
		t.Errorf("Authorization: got %q", gotToken)
	}
	if !bytes.Equal(gotBody, body) {
		t.Errorf("body mismatch over noise: got %q, want %q", gotBody, body)
	}
}
