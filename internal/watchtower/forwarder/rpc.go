package forwarder

import (
	"encoding/json"
	"log/slog"

	"github.com/aeddi/gno-watchtower/pkg/protocol"
)

// extractRPC converts a sentinel RPCPayload into VictoriaMetrics
// /api/v1/import lines.
//
// The sentinel collector polls tendermint/gno JSON-RPC endpoints, unwraps
// the JSON-RPC envelope (see internal/sentinel/rpc/client.go), and stores
// the inner `result` object under well-known keys in payload.Data. Each key
// is mapped to a small fixed set of gauges:
//
//	status               → sentinel_rpc_latest_block_height,
//	                       sentinel_rpc_catching_up (0|1),
//	                       sentinel_rpc_validator_voting_power (own)
//	net_info             → sentinel_rpc_peers
//	num_unconfirmed_txs  → sentinel_rpc_mempool_txs, sentinel_rpc_mempool_bytes
//	dump_consensus_state → sentinel_rpc_consensus_{height,round,step}
//	validators           → sentinel_rpc_validator_set_size,
//	                       sentinel_rpc_validator_set_total_power
//	block                → sentinel_rpc_block_num_txs
//
// Every emitted line carries the validator label. Malformed shapes and
// individual field parse failures log at Debug so persistent drift is grep-able.
// Aggregate-style metrics (validator_set_total_power) are all-or-nothing: if any
// entry fails to parse, the group is dropped rather than silently understated.
func extractRPC(validator string, payload protocol.RPCPayload) []vmLine {
	ts := payload.CollectedAt.UnixMilli()
	// 3 (status) + 1 (net_info) + 2 (mempool) + 3 (consensus) + 2 (validators) + 1 (block) = 12 max.
	lines := make([]vmLine, 0, 12)

	for key, raw := range payload.Data {
		switch key {
		case "status":
			lines = appendRPCStatus(lines, validator, ts, raw)
		case "net_info":
			lines = appendRPCNetInfo(lines, validator, ts, raw)
		case "num_unconfirmed_txs":
			lines = appendRPCMempool(lines, validator, ts, raw)
		case "dump_consensus_state":
			lines = appendRPCConsensus(lines, validator, ts, raw)
		case "validators":
			lines = appendRPCValidators(lines, validator, ts, raw)
		case "block":
			lines = appendRPCBlock(lines, validator, ts, raw)
		}
	}
	return lines
}

// decodeResult unmarshals raw into T. Returns (zero, false) on error with a
// Debug log so operators can grep for persistent shape drift without the
// extractors needing a logger parameter.
func decodeResult[T any](raw json.RawMessage, key, validator string) (T, bool) {
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		slog.Default().Debug("rpc: unmarshal failed", "key", key, "validator", validator, "err", err)
		var zero T
		return zero, false
	}
	return v, true
}

// intSample builds a single vmLine from a json.Number field, or returns
// (_, false) + Debug-logs if the number isn't parseable as int64. Callers
// use this to keep per-field partial-emission semantics while still leaving
// a grep-able trail when tendermint's numeric encoding drifts.
func intSample(name string, labels map[string]string, num json.Number, ts int64, key, validator string) (vmLine, bool) {
	n, err := num.Int64()
	if err != nil {
		slog.Default().Debug("rpc: int64 parse failed", "metric", name, "key", key, "validator", validator, "err", err)
		return vmLine{}, false
	}
	return vmSample(name, labels, float64(n), ts), true
}

type rpcStatus struct {
	SyncInfo struct {
		LatestBlockHeight json.Number `json:"latest_block_height"`
		CatchingUp        bool        `json:"catching_up"`
	} `json:"sync_info"`
	ValidatorInfo *struct {
		VotingPower json.Number `json:"voting_power"`
	} `json:"validator_info"`
}

func appendRPCStatus(lines []vmLine, validator string, ts int64, raw json.RawMessage) []vmLine {
	r, ok := decodeResult[rpcStatus](raw, "status", validator)
	if !ok {
		return lines
	}
	base := map[string]string{"validator": validator}
	if s, ok := intSample("sentinel_rpc_latest_block_height", base, r.SyncInfo.LatestBlockHeight, ts, "status", validator); ok {
		lines = append(lines, s)
	}
	lines = append(lines, vmSample("sentinel_rpc_catching_up", base, boolToFloat(r.SyncInfo.CatchingUp), ts))
	if r.ValidatorInfo != nil {
		if s, ok := intSample("sentinel_rpc_validator_voting_power", base, r.ValidatorInfo.VotingPower, ts, "status", validator); ok {
			lines = append(lines, s)
		}
	}
	return lines
}

type rpcNetInfo struct {
	NPeers json.Number `json:"n_peers"`
}

func appendRPCNetInfo(lines []vmLine, validator string, ts int64, raw json.RawMessage) []vmLine {
	r, ok := decodeResult[rpcNetInfo](raw, "net_info", validator)
	if !ok {
		return lines
	}
	if s, ok := intSample("sentinel_rpc_peers", map[string]string{"validator": validator}, r.NPeers, ts, "net_info", validator); ok {
		lines = append(lines, s)
	}
	return lines
}

type rpcMempool struct {
	NTxs       json.Number `json:"n_txs"`
	TotalBytes json.Number `json:"total_bytes"`
}

func appendRPCMempool(lines []vmLine, validator string, ts int64, raw json.RawMessage) []vmLine {
	r, ok := decodeResult[rpcMempool](raw, "num_unconfirmed_txs", validator)
	if !ok {
		return lines
	}
	base := map[string]string{"validator": validator}
	if s, ok := intSample("sentinel_rpc_mempool_txs", base, r.NTxs, ts, "num_unconfirmed_txs", validator); ok {
		lines = append(lines, s)
	}
	if s, ok := intSample("sentinel_rpc_mempool_bytes", base, r.TotalBytes, ts, "num_unconfirmed_txs", validator); ok {
		lines = append(lines, s)
	}
	return lines
}

type rpcConsensus struct {
	RoundState struct {
		// gnoland encodes height as a string, round/step as numbers; json.Number accepts both.
		Height json.Number `json:"height"`
		Round  json.Number `json:"round"`
		Step   json.Number `json:"step"`
	} `json:"round_state"`
}

func appendRPCConsensus(lines []vmLine, validator string, ts int64, raw json.RawMessage) []vmLine {
	r, ok := decodeResult[rpcConsensus](raw, "dump_consensus_state", validator)
	if !ok {
		return lines
	}
	base := map[string]string{"validator": validator}
	if s, ok := intSample("sentinel_rpc_consensus_height", base, r.RoundState.Height, ts, "dump_consensus_state", validator); ok {
		lines = append(lines, s)
	}
	if s, ok := intSample("sentinel_rpc_consensus_round", base, r.RoundState.Round, ts, "dump_consensus_state", validator); ok {
		lines = append(lines, s)
	}
	if s, ok := intSample("sentinel_rpc_consensus_step", base, r.RoundState.Step, ts, "dump_consensus_state", validator); ok {
		lines = append(lines, s)
	}
	return lines
}

type rpcValidators struct {
	Validators []struct {
		VotingPower json.Number `json:"voting_power"`
	} `json:"validators"`
}

func appendRPCValidators(lines []vmLine, validator string, ts int64, raw json.RawMessage) []vmLine {
	r, ok := decodeResult[rpcValidators](raw, "validators", validator)
	if !ok {
		return lines
	}
	// Aggregate must be all-or-nothing: a silently-skipped entry would emit a
	// total_power that disagrees with set_size, which is worse than no sample.
	var total int64
	for _, v := range r.Validators {
		p, err := v.VotingPower.Int64()
		if err != nil {
			slog.Default().Debug("rpc: validator voting_power parse failed, dropping aggregate",
				"validator", validator, "err", err)
			return lines
		}
		total += p
	}
	base := map[string]string{"validator": validator}
	return append(lines,
		vmSample("sentinel_rpc_validator_set_size", base, float64(len(r.Validators)), ts),
		vmSample("sentinel_rpc_validator_set_total_power", base, float64(total), ts),
	)
}

type rpcBlock struct {
	Block struct {
		Header struct {
			NumTxs json.Number `json:"num_txs"`
		} `json:"header"`
	} `json:"block"`
}

func appendRPCBlock(lines []vmLine, validator string, ts int64, raw json.RawMessage) []vmLine {
	r, ok := decodeResult[rpcBlock](raw, "block", validator)
	if !ok {
		return lines
	}
	if s, ok := intSample("sentinel_rpc_block_num_txs", map[string]string{"validator": validator}, r.Block.Header.NumTxs, ts, "block", validator); ok {
		lines = append(lines, s)
	}
	return lines
}

// boolToFloat maps bool → 0/1 for gauge emission of boolean health flags.
func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
