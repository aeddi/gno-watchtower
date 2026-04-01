// internal/sentinel/sender/sender.go
package sender

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/klauspost/compress/zstd"
)

type Sender struct {
	serverURL string
	token     string
	client    *http.Client
}

func New(serverURL, token string) *Sender {
	return &Sender{
		serverURL: serverURL,
		token:     token,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Send makes a single POST attempt with a JSON body. Returns an error for non-2xx responses.
func (s *Sender) Send(ctx context.Context, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return s.post(ctx, path, body, nil)
}

// SendCompressed makes a single POST attempt with a zstd-compressed JSON body.
// Sets Content-Encoding: zstd on the request.
func (s *Sender) SendCompressed(ctx context.Context, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	compressed, err := zstdCompress(body)
	if err != nil {
		return fmt.Errorf("compress: %w", err)
	}
	return s.post(ctx, path, compressed, map[string]string{"Content-Encoding": "zstd"})
}

// SendWithRetry retries Send up to maxAttempts times with exponential backoff.
// initialBackoff is the wait before the second attempt; it doubles each retry, capped at 30s.
func (s *Sender) SendWithRetry(ctx context.Context, path string, payload any, maxAttempts int, initialBackoff time.Duration) error {
	return retry(ctx, maxAttempts, initialBackoff, func() error {
		return s.Send(ctx, path, payload)
	})
}

// SendCompressedWithRetry retries SendCompressed up to maxAttempts times with exponential backoff.
func (s *Sender) SendCompressedWithRetry(ctx context.Context, path string, payload any, maxAttempts int, initialBackoff time.Duration) error {
	return retry(ctx, maxAttempts, initialBackoff, func() error {
		return s.SendCompressed(ctx, path, payload)
	})
}

// SendRaw makes a single POST attempt with raw bytes and a specific Content-Type.
// Use for non-JSON payloads such as protobuf (Content-Type: application/x-protobuf).
func (s *Sender) SendRaw(ctx context.Context, path string, body []byte, contentType string) error {
	return s.post(ctx, path, body, map[string]string{"Content-Type": contentType})
}

// SendRawWithRetry retries SendRaw up to maxAttempts times with exponential backoff.
func (s *Sender) SendRawWithRetry(ctx context.Context, path string, body []byte, contentType string, maxAttempts int, initialBackoff time.Duration) error {
	return retry(ctx, maxAttempts, initialBackoff, func() error {
		return s.SendRaw(ctx, path, body, contentType)
	})
}

// post executes a single HTTP POST. extraHeaders are applied after Content-Type and Authorization.
func (s *Sender) post(ctx context.Context, path string, body []byte, extraHeaders map[string]string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.serverURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.token)
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("post %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("post %s: unexpected status %d", path, resp.StatusCode)
	}
	return nil
}

// retry calls fn up to maxAttempts times with exponential backoff.
func retry(ctx context.Context, maxAttempts int, initialBackoff time.Duration, fn func() error) error {
	backoff := initialBackoff
	var lastErr error
	for i := 0; i < maxAttempts; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}
		if lastErr = fn(); lastErr == nil {
			return nil
		}
	}
	return fmt.Errorf("all %d attempts failed: %w", maxAttempts, lastErr)
}

// zstdCompress returns the zstd-compressed form of data.
func zstdCompress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := zstd.NewWriter(&buf)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(data); err != nil {
		w.Close()
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
