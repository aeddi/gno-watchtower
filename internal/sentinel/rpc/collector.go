// internal/sentinel/rpc/collector.go
package rpc

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/gnolang/val-companion/pkg/protocol"
)

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
	log          *slog.Logger
}

func NewCollector(client *Client, pollInterval, dumpInterval time.Duration, out chan<- protocol.RPCPayload, log *slog.Logger) *Collector {
	return &Collector{
		client:       client,
		delta:        NewDelta(),
		pollInterval: pollInterval,
		dumpInterval: dumpInterval,
		out:          out,
		log:          log.With("component", "rpc_collector"),
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
			// collect errors are transient; log and continue.
			if err := c.collect(ctx); err != nil {
				c.log.Error("collect failed", "err", err)
			}
		}
	}
}

func (c *Collector) collect(ctx context.Context) error {
	payload := protocol.RPCPayload{
		CollectedAt: time.Now().UTC(),
		Data:        make(map[string]json.RawMessage),
	}

	polled := []struct {
		key  string
		call func() (json.RawMessage, error)
	}{
		{"status", c.client.Status},
		{"net_info", c.client.NetInfo},
		{"num_unconfirmed_txs", c.client.NumUnconfirmedTxs},
	}

	var currentHeight int64
	var changed []string
	for _, p := range polled {
		raw, err := p.call()
		if err != nil {
			c.log.Warn("endpoint error", "endpoint", p.key, "err", err)
			continue
		}
		if p.key == "status" {
			currentHeight = c.parseHeight(raw)
		}
		if c.delta.Changed(p.key, raw) {
			payload.Data[p.key] = raw
			changed = append(changed, p.key)
		}
	}

	if time.Since(c.lastDump) >= c.dumpInterval {
		if raw, err := c.client.DumpConsensusState(); err == nil {
			if c.delta.Changed("dump_consensus_state", raw) {
				payload.Data["dump_consensus_state"] = raw
				changed = append(changed, "dump_consensus_state")
			}
		} else {
			c.log.Warn("endpoint error", "endpoint", "dump_consensus_state", "err", err)
		}
		c.lastDump = time.Now()
	}

	if currentHeight > c.lastHeight && currentHeight > 0 {
		c.lastHeight = currentHeight
		c.log.Info("new block", "height", currentHeight)

		if raw, err := c.client.Validators(currentHeight); err == nil {
			if c.delta.Changed("validators", raw) {
				payload.Data["validators"] = raw
				changed = append(changed, "validators")
			}
		}
		if raw, err := c.client.Block(currentHeight); err == nil {
			payload.Data["block"] = raw
			changed = append(changed, "block")
		}
		if raw, err := c.client.BlockResults(currentHeight); err == nil {
			payload.Data["block_results"] = raw
			changed = append(changed, "block_results")
		}
	}

	if len(changed) > 0 {
		c.log.Debug("poll", "changed", changed)
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
	h, err := s.SyncInfo.LatestBlockHeight.Int64()
	if err != nil {
		return 0
	}
	return h
}
