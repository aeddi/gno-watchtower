// internal/sentinel/rpc/collector.go
package rpc

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/aeddi/gno-watchtower/internal/sentinel/delta"
	"github.com/aeddi/gno-watchtower/pkg/protocol"
)

type statusResult struct {
	SyncInfo struct {
		LatestBlockHeight json.Number `json:"latest_block_height"`
	} `json:"sync_info"`
}

// Collector polls gnoland RPC endpoints and emits RPCPayloads to out.
// Unchanged responses (by hash) are omitted from the payload (delta).
// /block is always emitted when a new block is detected.
// /genesis is static per chain and fetched once at first successful poll.
//
// All fields (notably lastHeight, lastDump, genesisSent, genesisFailed) are
// read and written exclusively from collect(), which is itself called from a
// single goroutine driven by Run's ticker. No synchronisation needed; future
// readers must preserve this invariant.
type Collector struct {
	client        *Client
	delta         *delta.Delta
	pollInterval  time.Duration
	dumpInterval  time.Duration
	out           chan<- protocol.RPCPayload
	lastHeight    int64
	lastDump      time.Time
	genesisSent   bool
	genesisFailed int // # of consecutive failures; throttles log level
	log           *slog.Logger
}

func NewCollector(client *Client, pollInterval, dumpInterval time.Duration, out chan<- protocol.RPCPayload, log *slog.Logger) *Collector {
	return &Collector{
		client:       client,
		delta:        delta.NewDelta(),
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
			if err := c.collect(ctx); err != nil && ctx.Err() == nil {
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
		{"status", func() (json.RawMessage, error) { return c.client.Status(ctx) }},
		{"net_info", func() (json.RawMessage, error) { return c.client.NetInfo(ctx) }},
		{"num_unconfirmed_txs", func() (json.RawMessage, error) { return c.client.NumUnconfirmedTxs(ctx) }},
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
		// num_unconfirmed_txs is always included regardless of the delta so that
		// mempool_size is continuously sampled in VictoriaMetrics even when the
		// value stays at 0 (idle testnet / quiet validator).
		if p.key == "num_unconfirmed_txs" || c.delta.Changed(p.key, raw) {
			payload.Data[p.key] = raw
			changed = append(changed, p.key)
		}
	}

	if time.Since(c.lastDump) >= c.dumpInterval {
		if raw, err := c.client.DumpConsensusState(ctx); err == nil {
			if c.delta.Changed("dump_consensus_state", raw) {
				payload.Data["dump_consensus_state"] = raw
				changed = append(changed, "dump_consensus_state")
			}
			c.lastDump = time.Now()
		} else {
			c.log.Warn("endpoint error", "endpoint", "dump_consensus_state", "err", err)
		}
	}

	if currentHeight > c.lastHeight && currentHeight > 0 {
		c.lastHeight = currentHeight
		c.log.Info("new block", "height", currentHeight)

		if raw, err := c.client.Validators(ctx, currentHeight); err == nil {
			if c.delta.Changed("validators", raw) {
				payload.Data["validators"] = raw
				changed = append(changed, "validators")
			}
		} else {
			c.log.Warn("endpoint error", "endpoint", "validators", "err", err)
		}
		if raw, err := c.client.Block(ctx, currentHeight); err == nil {
			payload.Data["block"] = raw
			changed = append(changed, "block")
		} else {
			c.log.Warn("endpoint error", "endpoint", "block", "err", err)
		}
	}

	// /genesis is static per chain — fetch once and never again. Transient
	// failures retry each tick; to avoid log spam when a node persistently
	// can't reach /genesis, we emit Warn once then drop to Debug.
	if !c.genesisSent {
		raw, err := c.client.Genesis(ctx)
		switch {
		case err == nil:
			payload.Data["genesis"] = raw
			changed = append(changed, "genesis")
			c.genesisSent = true
		case c.genesisFailed == 0:
			c.log.Warn("endpoint error", "endpoint", "genesis", "err", err)
			c.genesisFailed++
		default:
			c.log.Debug("endpoint error", "endpoint", "genesis", "err", err, "attempt", c.genesisFailed+1)
			c.genesisFailed++
		}
	}

	if len(changed) > 0 {
		c.log.Debug("poll", "changed", changed)
	}

	if len(payload.Data) == 0 {
		return nil
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
