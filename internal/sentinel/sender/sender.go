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
	return s.post(ctx, path, body, "application/json", "")
}

// SendCompressed makes a single POST attempt with a zstd-compressed JSON body.
// Sets Content-Encoding: zstd on the request.
func (s *Sender) SendCompressed(ctx context.Context, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	compressed := ZstdCompress(body)
	return s.post(ctx, path, compressed, "application/json", "zstd")
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
	return s.post(ctx, path, body, contentType, "")
}

// SendRawWithRetry retries SendRaw up to maxAttempts times with exponential backoff.
func (s *Sender) SendRawWithRetry(ctx context.Context, path string, body []byte, contentType string, maxAttempts int, initialBackoff time.Duration) error {
	return retry(ctx, maxAttempts, initialBackoff, func() error {
		return s.SendRaw(ctx, path, body, contentType)
	})
}

// SendCompressedBytesWithRetry sends pre-compressed bytes with Content-Encoding: zstd.
// Use when the caller already holds the compressed bytes (e.g. to measure wire size).
func (s *Sender) SendCompressedBytesWithRetry(ctx context.Context, path string, body []byte, maxAttempts int, initialBackoff time.Duration) error {
	return retry(ctx, maxAttempts, initialBackoff, func() error {
		return s.post(ctx, path, body, "application/json", "zstd")
	})
}

// post executes a single HTTP POST.
// contentEncoding is set as Content-Encoding if non-empty.
func (s *Sender) post(ctx context.Context, path string, body []byte, contentType, contentEncoding string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.serverURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", contentType)
	if contentEncoding != "" {
		req.Header.Set("Content-Encoding", contentEncoding)
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
	for i := range maxAttempts {
		if i > 0 {
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
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

// zstdEncoder is a reusable stateless encoder; EncodeAll is safe for concurrent use.
var zstdEncoder = func() *zstd.Encoder {
	enc, err := zstd.NewWriter(nil)
	if err != nil {
		panic("init zstd encoder: " + err.Error())
	}
	return enc
}()

// ZstdCompress returns the zstd-compressed form of data.
func ZstdCompress(data []byte) []byte {
	return zstdEncoder.EncodeAll(data, make([]byte, 0, len(data)))
}
