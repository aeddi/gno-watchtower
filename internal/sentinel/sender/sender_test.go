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

	"github.com/klauspost/compress/zstd"

	"github.com/aeddi/gno-watchtower/internal/sentinel/sender"
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

func TestSender_SendCompressed_SetsHeader(t *testing.T) {
	var gotEncoding string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotEncoding = r.Header.Get("Content-Encoding")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := sender.New(srv.URL, "tok")
	payload := map[string]string{"level": "warn"}
	if err := s.SendCompressed(context.Background(), "/logs", payload); err != nil {
		t.Fatalf("SendCompressed: %v", err)
	}
	if gotEncoding != "zstd" {
		t.Errorf("Content-Encoding: got %q, want %q", gotEncoding, "zstd")
	}
	// Body must be valid zstd-compressed JSON.
	r, err := zstd.NewReader(bytes.NewReader(gotBody))
	if err != nil {
		t.Fatalf("zstd reader: %v", err)
	}
	defer r.Close()
	decompressed, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(decompressed, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["level"] != "warn" {
		t.Errorf("payload level: got %q, want %q", got["level"], "warn")
	}
}

func TestSender_SendCompressedWithRetry_RetriesOnFailure(t *testing.T) {
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

	s := sender.New(srv.URL, "tok")
	err := s.SendCompressedWithRetry(context.Background(), "/logs", map[string]string{}, 5, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("SendCompressedWithRetry: %v", err)
	}
	if attempts.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts.Load())
	}
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

	s := sender.New(srv.URL, "tok")
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

	s := sender.New(srv.URL, "tok")
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

func TestSender_SendWithRetry_RespectsRetryAfter(t *testing.T) {
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
	s := sender.New(srv.URL, "tok")
	start := time.Now()
	err := s.SendRawWithRetry(context.Background(), "/rpc", []byte("{}"), "application/json", 3, 500*time.Millisecond)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("SendRawWithRetry: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("expected 2 calls, got %d", calls.Load())
	}
	// Without Retry-After support, the retry would wait 500ms. With it, near-instant.
	if elapsed > 200*time.Millisecond {
		t.Errorf("expected Retry-After: 0 to skip backoff, elapsed %v", elapsed)
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

	s := sender.New(srv.URL, "tok")
	err := s.SendRawWithRetry(context.Background(), "/otlp", []byte("data"), "application/x-protobuf", 5, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("SendRawWithRetry: %v", err)
	}
	if attempts.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts.Load())
	}
}
