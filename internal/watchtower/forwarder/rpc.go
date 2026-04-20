package forwarder

import (
	"encoding/json"
	"log/slog"
	"strings"

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
	// status 4 + net_info 1 + mempool 2 + consensus 3 + validators 2 + block 1 = 13;
	// genesis one-shot adds 1 info + up to 6 consensus_params + N validators.
	// 32 covers a small validator set; runtime growth is cheap.
	lines := make([]vmLine, 0, 32)
	log := slog.Default()

	for key, raw := range payload.Data {
		switch key {
		case "status":
			lines = appendRPCStatus(lines, validator, ts, raw, log)
		case "net_info":
			lines = appendRPCNetInfo(lines, validator, ts, raw, log)
		case "num_unconfirmed_txs":
			lines = appendRPCMempool(lines, validator, ts, raw, log)
		case "dump_consensus_state":
			lines = appendRPCConsensus(lines, validator, ts, raw, log)
		case "validators":
			lines = appendRPCValidators(lines, validator, ts, raw, log)
		case "block":
			lines = appendRPCBlock(lines, validator, ts, raw, log)
		case "genesis":
			lines = appendRPCGenesis(lines, validator, ts, raw, log)
		}
	}
	return lines
}

// decodeResult unmarshals raw into T. The raw payload is the inner `result`
// object from the gnoland JSON-RPC envelope; the envelope itself is stripped
// by the sentinel's rpc.Client.get — see internal/sentinel/rpc/client.go.
// Returns (zero, false) on error with a Debug log so operators can grep for
// persistent shape drift.
func decodeResult[T any](raw json.RawMessage, key, validator string, log *slog.Logger) (T, bool) {
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		log.Debug("rpc: unmarshal failed", "key", key, "validator", validator, "err", err)
		var zero T
		return zero, false
	}
	return v, true
}

// intSample builds a single vmLine from a json.Number field, or returns
// (_, false) + Debug-logs if the number isn't parseable as int64. Callers
// use this to keep per-field partial-emission semantics while still leaving
// a grep-able trail when tendermint's numeric encoding drifts.
func intSample(name string, labels map[string]string, num json.Number, ts int64, key, validator string, log *slog.Logger) (vmLine, bool) {
	n, err := num.Int64()
	if err != nil {
		log.Debug("rpc: int64 parse failed", "metric", name, "key", key, "validator", validator, "err", err)
		return vmLine{}, false
	}
	return vmSample(name, labels, float64(n), ts), true
}

type rpcStatus struct {
	NodeInfo struct {
		Moniker string `json:"moniker"`
		Network string `json:"network"` // chain_id
		Version string `json:"version"` // gnoland build version (e.g. master.12345+abcdef0)
	} `json:"node_info"`
	SyncInfo struct {
		LatestBlockHeight json.Number `json:"latest_block_height"`
		CatchingUp        bool        `json:"catching_up"`
	} `json:"sync_info"`
	ValidatorInfo *struct {
		Address     string      `json:"address"`
		VotingPower json.Number `json:"voting_power"`
	} `json:"validator_info"`
}

func appendRPCStatus(lines []vmLine, validator string, ts int64, raw json.RawMessage, log *slog.Logger) []vmLine {
	r, ok := decodeResult[rpcStatus](raw, "status", validator, log)
	if !ok {
		return lines
	}
	base := map[string]string{"validator": validator}
	if s, ok := intSample("sentinel_rpc_latest_block_height", base, r.SyncInfo.LatestBlockHeight, ts, "status", validator, log); ok {
		lines = append(lines, s)
	}
	lines = append(lines, vmSample("sentinel_rpc_catching_up", base, boolToFloat(r.SyncInfo.CatchingUp), ts))
	if r.ValidatorInfo != nil {
		// Own voting power carries the validator's on-chain address as a label
		// so consensus-quorum dashboards can join this against the set-wide
		// sentinel_rpc_validator_set_power on (address).
		vpLabels := map[string]string{"validator": validator, "address": r.ValidatorInfo.Address}
		if s, ok := intSample("sentinel_rpc_validator_voting_power", vpLabels, r.ValidatorInfo.VotingPower, ts, "status", validator, log); ok {
			lines = append(lines, s)
		}
		// sentinel_validator_online is a presence signal: we emit it only when
		// the sentinel is reporting AND the node is caught up. Offline /
		// catching-up nodes produce no sample, and the series ages out of the
		// VM staleness window — "missing" == "not online". The consensus-
		// quorum dashboard subtracts the offline nodes' power from total to
		// compute the live active voting power.
		if r.ValidatorInfo.Address != "" && !r.SyncInfo.CatchingUp {
			lines = append(lines, vmSample("sentinel_validator_online",
				map[string]string{"address": r.ValidatorInfo.Address}, 1, ts))
		}
	}

	// Build-info gauge: Prometheus-style info metric carrying identifying
	// labels at value 1. Operators dashboard these to spot version drift.
	// Missing fields degrade to empty-label series rather than being dropped —
	// a moniker-less node is still a node and still deserves a series.
	infoLabels := map[string]string{
		"validator": validator,
		"chain_id":  r.NodeInfo.Network,
		"moniker":   r.NodeInfo.Moniker,
		"version":   r.NodeInfo.Version,
	}
	lines = append(lines, vmSample("sentinel_node_build_info", infoLabels, 1, ts))
	return lines
}

type rpcNetInfo struct {
	NPeers json.Number `json:"n_peers"`
}

func appendRPCNetInfo(lines []vmLine, validator string, ts int64, raw json.RawMessage, log *slog.Logger) []vmLine {
	r, ok := decodeResult[rpcNetInfo](raw, "net_info", validator, log)
	if !ok {
		return lines
	}
	if s, ok := intSample("sentinel_rpc_peers", map[string]string{"validator": validator}, r.NPeers, ts, "net_info", validator, log); ok {
		lines = append(lines, s)
	}
	return lines
}

type rpcMempool struct {
	NTxs       json.Number `json:"n_txs"`
	TotalBytes json.Number `json:"total_bytes"`
}

func appendRPCMempool(lines []vmLine, validator string, ts int64, raw json.RawMessage, log *slog.Logger) []vmLine {
	r, ok := decodeResult[rpcMempool](raw, "num_unconfirmed_txs", validator, log)
	if !ok {
		return lines
	}
	base := map[string]string{"validator": validator}
	if s, ok := intSample("sentinel_rpc_mempool_txs", base, r.NTxs, ts, "num_unconfirmed_txs", validator, log); ok {
		lines = append(lines, s)
	}
	if s, ok := intSample("sentinel_rpc_mempool_bytes", base, r.TotalBytes, ts, "num_unconfirmed_txs", validator, log); ok {
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

func appendRPCConsensus(lines []vmLine, validator string, ts int64, raw json.RawMessage, log *slog.Logger) []vmLine {
	r, ok := decodeResult[rpcConsensus](raw, "dump_consensus_state", validator, log)
	if !ok {
		return lines
	}
	base := map[string]string{"validator": validator}
	if s, ok := intSample("sentinel_rpc_consensus_height", base, r.RoundState.Height, ts, "dump_consensus_state", validator, log); ok {
		lines = append(lines, s)
	}
	if s, ok := intSample("sentinel_rpc_consensus_round", base, r.RoundState.Round, ts, "dump_consensus_state", validator, log); ok {
		lines = append(lines, s)
	}
	if s, ok := intSample("sentinel_rpc_consensus_step", base, r.RoundState.Step, ts, "dump_consensus_state", validator, log); ok {
		lines = append(lines, s)
	}
	return lines
}

type rpcValidators struct {
	Validators []struct {
		Address     string      `json:"address"`
		VotingPower json.Number `json:"voting_power"`
	} `json:"validators"`
}

func appendRPCValidators(lines []vmLine, validator string, ts int64, raw json.RawMessage, log *slog.Logger) []vmLine {
	r, ok := decodeResult[rpcValidators](raw, "validators", validator, log)
	if !ok {
		return lines
	}
	// Aggregate must be all-or-nothing: a silently-skipped entry would emit a
	// total_power that disagrees with set_size, which is worse than no sample.
	// Collect per-member powers alongside the aggregate so we can emit both
	// in lockstep if every entry parses.
	type member struct {
		address string
		power   int64
	}
	members := make([]member, 0, len(r.Validators))
	var total int64
	for _, v := range r.Validators {
		p, err := v.VotingPower.Int64()
		if err != nil {
			log.Debug("rpc: validator voting_power parse failed, dropping aggregate",
				"validator", validator, "err", err)
			return lines
		}
		members = append(members, member{address: v.Address, power: p})
		total += p
	}
	base := map[string]string{"validator": validator}
	lines = append(lines,
		vmSample("sentinel_rpc_validator_set_size", base, float64(len(r.Validators)), ts),
		vmSample("sentinel_rpc_validator_set_total_power", base, float64(total), ts),
	)
	// Per-member gauge: one series per validator-in-set per reporter. The
	// dashboard dedupes across reporters via `max by (address)` and joins on
	// sentinel_validator_online{address} to compute active voting power.
	for _, m := range members {
		if m.address == "" {
			continue
		}
		lines = append(lines, vmSample("sentinel_rpc_validator_set_power",
			map[string]string{"validator": validator, "address": m.address},
			float64(m.power), ts))
	}
	return lines
}

type rpcBlock struct {
	Block struct {
		Header struct {
			NumTxs json.Number `json:"num_txs"`
		} `json:"header"`
	} `json:"block"`
}

func appendRPCBlock(lines []vmLine, validator string, ts int64, raw json.RawMessage, log *slog.Logger) []vmLine {
	r, ok := decodeResult[rpcBlock](raw, "block", validator, log)
	if !ok {
		return lines
	}
	if s, ok := intSample("sentinel_rpc_block_num_txs", map[string]string{"validator": validator}, r.Block.Header.NumTxs, ts, "block", validator, log); ok {
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

// rpcGenesisBlockParams mirrors the nested consensus_params.Block object in
// /genesis. Fields use gnoland's PascalCase (see tm2/pkg/bft/types/genesis.go).
type rpcGenesisBlockParams struct {
	MaxTxBytes    json.Number `json:"MaxTxBytes"`
	MaxDataBytes  json.Number `json:"MaxDataBytes"`
	MaxBlockBytes json.Number `json:"MaxBlockBytes"`
	MaxGas        json.Number `json:"MaxGas"`
	TimeIotaMS    json.Number `json:"TimeIotaMS"`
}

type rpcGenesisValidatorParams struct {
	PubKeyTypeURLs []string `json:"PubKeyTypeURLs"`
}

type rpcGenesisConsensusParams struct {
	Block     rpcGenesisBlockParams     `json:"Block"`
	Validator rpcGenesisValidatorParams `json:"Validator"`
}

type rpcGenesisValidator struct {
	Address string `json:"address"`
	PubKey  struct {
		Value string `json:"value"`
	} `json:"pub_key"`
	Power json.Number `json:"power"`
	Name  string      `json:"name"`
}

type rpcGenesisDoc struct {
	GenesisTime     string                    `json:"genesis_time"`
	ChainID         string                    `json:"chain_id"`
	AppHash         json.RawMessage           `json:"app_hash"`
	ConsensusParams rpcGenesisConsensusParams `json:"consensus_params"`
	Validators      []rpcGenesisValidator     `json:"validators"`
}

// rpcGenesisResult is the outer shape of /genesis after the JSON-RPC envelope
// has been stripped in the sentinel's rpc.Client — i.e. `{"genesis": {...}}`.
type rpcGenesisResult struct {
	Genesis rpcGenesisDoc `json:"genesis"`
}

// normalizeAppHash handles the three shapes gnoland may emit for app_hash in
// /genesis: a quoted hex string, an empty string, or literal JSON null. We
// collapse the last two to the empty string so the dashboard label is either
// a real hex value or absent (VM drops empty labels on ingest).
func normalizeAppHash(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Try to unmarshal as a string first — handles the quoted case correctly
	// regardless of any whitespace-padding from the JSON encoder.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	// Fallback: raw token is null/unknown; treat as absent.
	return ""
}

// appendRPCGenesis emits the static-per-chain genesis info. Each field becomes
// a label on an *_info gauge (value=1), matching the Prometheus info-metric
// idiom — the operator dashboards compare these across the fleet and any
// value drift stands out as a column with a different string.
func appendRPCGenesis(lines []vmLine, validator string, ts int64, raw json.RawMessage, log *slog.Logger) []vmLine {
	r, ok := decodeResult[rpcGenesisResult](raw, "genesis", validator, log)
	if !ok {
		return lines
	}
	g := r.Genesis
	appHash := normalizeAppHash(g.AppHash)

	// Top-level genesis info: one series per validator carrying chain identity.
	lines = append(lines, vmSample("sentinel_genesis_info", map[string]string{
		"validator":    validator,
		"chain_id":     g.ChainID,
		"genesis_time": g.GenesisTime,
		"app_hash":     appHash,
	}, 1, ts))

	// Consensus params: flat "param=value" rows so the dashboard can lay them
	// out as a matrix of (param × validator) cells. String values preserve
	// unit hints (e.g. ms) the operator might compare at a glance.
	type cp struct {
		name, value string
	}
	params := []cp{
		{"block.max_tx_bytes", g.ConsensusParams.Block.MaxTxBytes.String()},
		{"block.max_data_bytes", g.ConsensusParams.Block.MaxDataBytes.String()},
		{"block.max_block_bytes", g.ConsensusParams.Block.MaxBlockBytes.String()},
		{"block.max_gas", g.ConsensusParams.Block.MaxGas.String()},
		{"block.time_iota_ms", g.ConsensusParams.Block.TimeIotaMS.String()},
		{"validator.pub_key_types", strings.Join(g.ConsensusParams.Validator.PubKeyTypeURLs, ",")},
	}
	for _, p := range params {
		if p.value == "" {
			continue
		}
		lines = append(lines, vmSample("sentinel_genesis_consensus_param", map[string]string{
			"validator": validator,
			"param":     p.name,
			"value":     p.value,
		}, 1, ts))
	}

	// Genesis validator set: one series per genesis validator per reporter.
	// Cardinality is bounded by (fleet size × genesis validator count).
	for _, v := range g.Validators {
		lines = append(lines, vmSample("sentinel_genesis_validator", map[string]string{
			"validator": validator,
			"address":   v.Address,
			"pub_key":   v.PubKey.Value,
			"power":     v.Power.String(),
			"name":      v.Name,
		}, 1, ts))
	}
	return lines
}
