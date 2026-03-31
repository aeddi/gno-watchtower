// internal/sentinel/rpc/collector.go
package rpc

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/gnolang/val-companion/pkg/protocol"
)

// statusResult is used only to extract the latest block height.
// gnoland returns int64 heights as JSON strings (Tendermint convention).
// Verify this against a live node if behavior is unexpected.
type statusResult struct {
	SyncInfo struct {
		LatestBlockHeight json.Number `json:"latest_block_height"`
	} `json:"sync_info"`
}

// Collector polls gnoland RPC endpoints and emits RPCPayloads to out.
// Unchanged responses (by hash) are omitted from the payload (delta).
// /block and /block_results are always emitted when a new block is detected.
type Collector struct {
	client       *Client
	delta        *Delta
	pollInterval time.Duration
	dumpInterval time.Duration
	out          chan<- protocol.RPCPayload
	lastHeight   int64
	lastDump     time.Time
	logf         func(string, ...any)
}

func NewCollector(client *Client, pollInterval, dumpInterval time.Duration, out chan<- protocol.RPCPayload, logf func(string, ...any)) *Collector {
	return &Collector{
		client:       client,
		delta:        NewDelta(),
		pollInterval: pollInterval,
		dumpInterval: dumpInterval,
		out:          out,
		logf:         logf,
	}
}

func (c *Collector) Run(ctx context.Context) error {
	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := c.collect(ctx); err != nil {
				// Log and continue — transient RPC errors should not stop the collector.
				c.logf("collect error: %v\n", err)
			}
		}
	}
}

func (c *Collector) collect(ctx context.Context) error {
	payload := protocol.RPCPayload{
		CollectedAt: time.Now().UTC(),
		Data:        make(map[string]json.RawMessage),
	}

	// Always poll these endpoints; only include in payload if changed.
	polled := []struct {
		key  string
		call func() (json.RawMessage, error)
	}{
		{"status", c.client.Status},
		{"net_info", c.client.NetInfo},
		{"num_unconfirmed_txs", c.client.NumUnconfirmedTxs},
	}

	var currentHeight int64
	for _, p := range polled {
		raw, err := p.call()
		if err != nil {
			continue
		}
		if p.key == "status" {
			currentHeight = c.parseHeight(raw)
		}
		if c.delta.Changed(p.key, raw) {
			payload.Data[p.key] = raw
		}
	}

	// dump_consensus_state: poll on its own interval.
	if time.Since(c.lastDump) >= c.dumpInterval {
		if raw, err := c.client.DumpConsensusState(); err == nil {
			if c.delta.Changed("dump_consensus_state", raw) {
				payload.Data["dump_consensus_state"] = raw
			}
		}
		c.lastDump = time.Now()
	}

	// Per-block endpoints: triggered when height advances.
	if currentHeight > c.lastHeight && currentHeight > 0 {
		c.lastHeight = currentHeight

		if raw, err := c.client.Validators(currentHeight); err == nil {
			if c.delta.Changed("validators", raw) {
				payload.Data["validators"] = raw
			}
		}
		// Block and block_results are always sent (each block is unique).
		if raw, err := c.client.Block(currentHeight); err == nil {
			payload.Data["block"] = raw
		}
		if raw, err := c.client.BlockResults(currentHeight); err == nil {
			payload.Data["block_results"] = raw
		}
	}

	select {
	case c.out <- payload:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func (c *Collector) parseHeight(raw json.RawMessage) int64 {
	var s statusResult
	if err := json.Unmarshal(raw, &s); err != nil {
		return 0
	}
	h, err := strconv.ParseInt(s.SyncInfo.LatestBlockHeight.String(), 10, 64)
	if err != nil {
		return 0
	}
	return h
}
