package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// rpcEnvelope is the JSON-RPC 2.0 response wrapper returned by gnoland.
// NOTE: verify the exact response format against a live gnoland node before use.
// Heights are typically returned as strings in Tendermint-based chains.
type rpcEnvelope struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) get(ctx context.Context, path string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build request %s: %w", path, err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get %s: unexpected status %d", path, resp.StatusCode)
	}

	var env rpcEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	if env.Error != nil {
		return nil, env.Error
	}
	return env.Result, nil
}

func (c *Client) Status(ctx context.Context) (json.RawMessage, error) {
	return c.get(ctx, "/status")
}
func (c *Client) NetInfo(ctx context.Context) (json.RawMessage, error) {
	return c.get(ctx, "/net_info")
}
func (c *Client) NumUnconfirmedTxs(ctx context.Context) (json.RawMessage, error) {
	return c.get(ctx, "/num_unconfirmed_txs")
}
func (c *Client) DumpConsensusState(ctx context.Context) (json.RawMessage, error) {
	return c.get(ctx, "/dump_consensus_state")
}
func (c *Client) Validators(ctx context.Context, height int64) (json.RawMessage, error) {
	return c.get(ctx, fmt.Sprintf("/validators?height=%d", height))
}
func (c *Client) Block(ctx context.Context, height int64) (json.RawMessage, error) {
	return c.get(ctx, fmt.Sprintf("/block?height=%d", height))
}
func (c *Client) Genesis(ctx context.Context) (json.RawMessage, error) {
	return c.get(ctx, "/genesis")
}
