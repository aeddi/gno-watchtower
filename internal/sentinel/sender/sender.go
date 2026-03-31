// internal/sentinel/sender/sender.go
package sender

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
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

// Send makes a single POST attempt. Returns an error for non-2xx responses.
func (s *Sender) Send(ctx context.Context, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.serverURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.token)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("post %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("post %s: unexpected status %d", path, resp.StatusCode)
	}
	return nil
}

// SendWithRetry retries Send up to maxAttempts times with exponential backoff.
// initialBackoff is the wait before the second attempt; it doubles each retry, capped at 30s.
func (s *Sender) SendWithRetry(ctx context.Context, path string, payload any, maxAttempts int, initialBackoff time.Duration) error {
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
		if lastErr = s.Send(ctx, path, payload); lastErr == nil {
			return nil
		}
	}
	return fmt.Errorf("all %d attempts failed: %w", maxAttempts, lastErr)
}
