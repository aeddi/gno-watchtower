// internal/sentinel/sender/sender.go
package sender

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/klauspost/compress/zstd"

	"github.com/aeddi/gno-watchtower/pkg/noise"
)

type Sender struct {
	serverURL string
	token     string
	client    *http.Client
}

// New builds a Sender. When serverURL uses the noise:// scheme, noiseCfg must
// be non-nil and the sender routes requests through a Noise-wrapped
// net.Conn; otherwise standard HTTPS is used.
//
// For noise://, the URL seen by the http.Client is internally rewritten to
// http:// — the transport layer is encrypted by Noise, so the HTTP layer runs
// in plaintext over the authenticated stream. The URL scheme is a routing
// hint, nothing more; the real host/port in the URL still drives which TCP
// peer the sender connects to.
func New(serverURL, token string, noiseCfg *noise.Config) (*Sender, error) {
	if strings.HasPrefix(serverURL, "noise://") {
		if noiseCfg == nil {
			return nil, fmt.Errorf("server.url is noise:// but no beacon keypair was configured")
		}
		// Rewrite noise://host:port/path → http://host:port/path for the
		// http.Client; the Transport below gives it a Noise-wrapped conn.
		rewritten := "http://" + strings.TrimPrefix(serverURL, "noise://")
		cfg := *noiseCfg // copy so the caller's struct isn't retained
		transport := &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return noise.Dial(ctx, network, addr, cfg)
			},
			// Reuse one persistent Noise session across posts — handshake is
			// amortised over all the sentinel's traffic.
			MaxIdleConns:        4,
			MaxIdleConnsPerHost: 2,
			IdleConnTimeout:     5 * time.Minute,
			DisableCompression:  true, // bodies are already handled by sender (zstd/plain)
		}
		return &Sender{
			serverURL: rewritten,
			token:     token,
			client:    &http.Client{Transport: transport, Timeout: 30 * time.Second},
		}, nil
	}
	return &Sender{
		serverURL: serverURL,
		token:     token,
		client:    &http.Client{Timeout: 30 * time.Second},
	}, nil
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

// retryAfterError is returned by post when the server responds 429 with a Retry-After header.
type retryAfterError struct {
	wait time.Duration
	base error
}

func (e *retryAfterError) Error() string { return e.base.Error() }
func (e *retryAfterError) Unwrap() error { return e.base }

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
	if resp.StatusCode == http.StatusTooManyRequests {
		_, _ = io.Copy(io.Discard, resp.Body)
		baseErr := fmt.Errorf("post %s: rate limited (429)", path)
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil {
				wait := time.Duration(secs) * time.Second
				if wait > 30*time.Second {
					wait = 30 * time.Second
				}
				return &retryAfterError{wait: wait, base: baseErr}
			}
		}
		return baseErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("post %s: unexpected status %d", path, resp.StatusCode)
	}
	return nil
}

// retry calls fn up to maxAttempts times with exponential backoff.
// If fn returns a retryAfterError, the specified wait overrides the exponential backoff.
func retry(ctx context.Context, maxAttempts int, initialBackoff time.Duration, fn func() error) error {
	backoff := initialBackoff
	var lastErr error
	skipBackoff := false
	for i := range maxAttempts {
		if i > 0 && !skipBackoff {
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
		skipBackoff = false
		if lastErr = fn(); lastErr == nil {
			return nil
		}
		var raErr *retryAfterError
		if errors.As(lastErr, &raErr) {
			if raErr.wait == 0 {
				skipBackoff = true
				continue
			}
			timer := time.NewTimer(raErr.wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			skipBackoff = true
		}
	}
	return fmt.Errorf("all %d attempts failed: %w", maxAttempts, lastErr)
}

// zstdEncoder is a reusable stateless encoder; EncodeAll is safe for concurrent use.
// Built lazily on first use via sync.OnceValue so package initialisation stays
// free of the zstd library's setup cost for callers that never compress.
var zstdEncoder = sync.OnceValue(func() *zstd.Encoder {
	enc, err := zstd.NewWriter(nil)
	if err != nil {
		panic("init zstd encoder: " + err.Error())
	}
	return enc
})

// ZstdCompress returns the zstd-compressed form of data.
func ZstdCompress(data []byte) []byte {
	return zstdEncoder().EncodeAll(data, make([]byte, 0, len(data)))
}
