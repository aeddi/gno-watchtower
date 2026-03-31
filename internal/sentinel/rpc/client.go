package rpc

import (
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
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) get(path string) (json.RawMessage, error) {
	resp, err := c.http.Get(c.baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", path, err)
	}
	defer resp.Body.Close()

	var env rpcEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	if env.Error != nil {
		return nil, env.Error
	}
	return env.Result, nil
}

func (c *Client) Status() (json.RawMessage, error)            { return c.get("/status") }
func (c *Client) NetInfo() (json.RawMessage, error)           { return c.get("/net_info") }
func (c *Client) NumUnconfirmedTxs() (json.RawMessage, error) { return c.get("/num_unconfirmed_txs") }
func (c *Client) DumpConsensusState() (json.RawMessage, error) {
	return c.get("/dump_consensus_state")
}
func (c *Client) Validators(height int64) (json.RawMessage, error) {
	return c.get(fmt.Sprintf("/validators?height=%d", height))
}
func (c *Client) Block(height int64) (json.RawMessage, error) {
	return c.get(fmt.Sprintf("/block?height=%d", height))
}
func (c *Client) BlockResults(height int64) (json.RawMessage, error) {
	return c.get(fmt.Sprintf("/block_results?height=%d", height))
}
