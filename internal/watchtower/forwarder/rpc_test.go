package forwarder

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/aeddi/gno-watchtower/pkg/protocol"
)

// ---- Fixtures matching the real tendermint / gno RPC JSON shapes.
//
// Note: the sentinel's rpc.Client unwraps the JSON-RPC {"jsonrpc":...,"result":...}
// envelope before storing the body in payload.Data, so fixtures here are the
// inner `result` objects — not full envelopes.

const (
	statusJSON = `{
		"sync_info":{"latest_block_height":"4478","catching_up":false},
		"validator_info":{"address":"g1self","voting_power":"1"}
	}`
	statusCatchingUpJSON = `{
		"sync_info":{"latest_block_height":"100","catching_up":true},
		"validator_info":{"address":"g1self","voting_power":"1"}
	}`
	netInfoJSON    = `{"n_peers":"3"}`
	mempoolJSON    = `{"n_txs":"2","total":"2","total_bytes":"512"}`
	consensusJSON  = `{"round_state":{"height":"4478","round":0,"step":1}}`
	validatorsJSON = `{
		"block_height":"4478",
		"validators":[
			{"address":"g1a","voting_power":"1"},
			{"address":"g1b","voting_power":"2"},
			{"address":"g1c","voting_power":"5"}
		]
	}`
	blockJSON = `{"block":{"header":{"height":"4478","num_txs":"3","time":"2026-04-19T21:00:00Z","proposer_address":"g1a"}}}`
)

func rpcPayload(keys map[string]string) protocol.RPCPayload {
	data := make(map[string]json.RawMessage, len(keys))
	for k, v := range keys {
		data[k] = json.RawMessage(v)
	}
	return protocol.RPCPayload{CollectedAt: collectedAt(), Data: data}
}

func TestExtractRPC_EmptyPayload(t *testing.T) {
	lines := extractRPC("node-1", rpcPayload(nil))
	if len(lines) != 0 {
		t.Errorf("extractRPC(empty) = %d lines, want 0", len(lines))
	}
}

func TestExtractRPC_Status(t *testing.T) {
	lines := extractRPC("node-1", rpcPayload(map[string]string{"status": statusJSON}))
	want := []string{
		"sentinel_rpc_catching_up",
		"sentinel_rpc_latest_block_height",
		"sentinel_rpc_validator_voting_power",
	}
	if got := metricNames(t, lines); !slices.Equal(got, want) {
		t.Fatalf("status metric names = %v, want %v", got, want)
	}
	h := findLine(t, lines, map[string]string{"__name__": "sentinel_rpc_latest_block_height"})
	if h == nil || h.Values[0] != 4478 {
		t.Errorf("height = %v, want 4478", h)
	}
	cu := findLine(t, lines, map[string]string{"__name__": "sentinel_rpc_catching_up"})
	if cu == nil || cu.Values[0] != 0 {
		t.Errorf("catching_up = %v, want 0 (false)", cu)
	}
	vp := findLine(t, lines, map[string]string{"__name__": "sentinel_rpc_validator_voting_power"})
	if vp == nil || vp.Values[0] != 1 {
		t.Errorf("own voting_power = %v, want 1", vp)
	}
}

func TestExtractRPC_Status_CatchingUpTrue(t *testing.T) {
	lines := extractRPC("node-1", rpcPayload(map[string]string{"status": statusCatchingUpJSON}))
	cu := findLine(t, lines, map[string]string{"__name__": "sentinel_rpc_catching_up"})
	if cu == nil || cu.Values[0] != 1 {
		t.Errorf("catching_up (true) = %v, want 1", cu)
	}
}

func TestExtractRPC_NetInfo(t *testing.T) {
	lines := extractRPC("node-1", rpcPayload(map[string]string{"net_info": netInfoJSON}))
	if got := metricNames(t, lines); !slices.Equal(got, []string{"sentinel_rpc_peers"}) {
		t.Fatalf("net_info metric names = %v", got)
	}
	if lines[0].Values[0] != 3 {
		t.Errorf("peers = %v, want 3", lines[0].Values[0])
	}
}

func TestExtractRPC_Mempool(t *testing.T) {
	lines := extractRPC("node-1", rpcPayload(map[string]string{"num_unconfirmed_txs": mempoolJSON}))
	want := []string{"sentinel_rpc_mempool_bytes", "sentinel_rpc_mempool_txs"}
	if got := metricNames(t, lines); !slices.Equal(got, want) {
		t.Fatalf("mempool metric names = %v, want %v", got, want)
	}
	txs := findLine(t, lines, map[string]string{"__name__": "sentinel_rpc_mempool_txs"})
	if txs == nil || txs.Values[0] != 2 {
		t.Errorf("mempool txs = %v, want 2", txs)
	}
	b := findLine(t, lines, map[string]string{"__name__": "sentinel_rpc_mempool_bytes"})
	if b == nil || b.Values[0] != 512 {
		t.Errorf("mempool bytes = %v, want 512", b)
	}
}

func TestExtractRPC_ConsensusState(t *testing.T) {
	lines := extractRPC("node-1", rpcPayload(map[string]string{"dump_consensus_state": consensusJSON}))
	want := []string{
		"sentinel_rpc_consensus_height",
		"sentinel_rpc_consensus_round",
		"sentinel_rpc_consensus_step",
	}
	if got := metricNames(t, lines); !slices.Equal(got, want) {
		t.Fatalf("consensus metric names = %v, want %v", got, want)
	}
	h := findLine(t, lines, map[string]string{"__name__": "sentinel_rpc_consensus_height"})
	if h == nil || h.Values[0] != 4478 {
		t.Errorf("consensus height = %v, want 4478", h)
	}
	r := findLine(t, lines, map[string]string{"__name__": "sentinel_rpc_consensus_round"})
	if r == nil || r.Values[0] != 0 {
		t.Errorf("round = %v, want 0", r)
	}
	s := findLine(t, lines, map[string]string{"__name__": "sentinel_rpc_consensus_step"})
	if s == nil || s.Values[0] != 1 {
		t.Errorf("step = %v, want 1", s)
	}
}

func TestExtractRPC_Validators(t *testing.T) {
	lines := extractRPC("node-1", rpcPayload(map[string]string{"validators": validatorsJSON}))
	want := []string{
		"sentinel_rpc_validator_set_size",
		"sentinel_rpc_validator_set_total_power",
	}
	if got := metricNames(t, lines); !slices.Equal(got, want) {
		t.Fatalf("validators metric names = %v, want %v", got, want)
	}
	n := findLine(t, lines, map[string]string{"__name__": "sentinel_rpc_validator_set_size"})
	if n == nil || n.Values[0] != 3 {
		t.Errorf("validator set size = %v, want 3", n)
	}
	p := findLine(t, lines, map[string]string{"__name__": "sentinel_rpc_validator_set_total_power"})
	if p == nil || p.Values[0] != 8 {
		t.Errorf("total voting power = %v, want 8 (1+2+5)", p)
	}
}

func TestExtractRPC_Block(t *testing.T) {
	lines := extractRPC("node-1", rpcPayload(map[string]string{"block": blockJSON}))
	if got := metricNames(t, lines); !slices.Equal(got, []string{"sentinel_rpc_block_num_txs"}) {
		t.Fatalf("block metric names = %v", got)
	}
	if lines[0].Values[0] != 3 {
		t.Errorf("block num_txs = %v, want 3", lines[0].Values[0])
	}
}

func TestExtractRPC_MultipleKeys(t *testing.T) {
	lines := extractRPC("node-1", rpcPayload(map[string]string{
		"status":               statusJSON,
		"net_info":             netInfoJSON,
		"num_unconfirmed_txs":  mempoolJSON,
		"dump_consensus_state": consensusJSON,
		"validators":           validatorsJSON,
		"block":                blockJSON,
	}))
	// 3 + 1 + 2 + 3 + 2 + 1 = 12
	if len(lines) != 12 {
		t.Errorf("got %d lines, want 12", len(lines))
	}
	for _, l := range lines {
		if l.Metric["validator"] != "node-1" {
			t.Errorf("%s: validator = %q, want node-1", l.Metric["__name__"], l.Metric["validator"])
		}
	}
}

func TestExtractRPC_MalformedJSON_SkipsKey(t *testing.T) {
	lines := extractRPC("node-1", rpcPayload(map[string]string{
		"status":   statusJSON,
		"net_info": `not-json`,
	}))
	// status still parses, net_info is dropped.
	for _, l := range lines {
		if l.Metric["__name__"] == "sentinel_rpc_peers" {
			t.Errorf("malformed net_info should not emit peers metric")
		}
	}
	if len(lines) == 0 {
		t.Error("no lines emitted; status should still parse")
	}
}

func TestExtractRPC_UnknownKey_Ignored(t *testing.T) {
	lines := extractRPC("node-1", rpcPayload(map[string]string{"block_results": `{"result":{}}`}))
	if len(lines) != 0 {
		t.Errorf("unknown key produced %d lines, want 0", len(lines))
	}
}

func TestExtractRPC_Validators_AggregateDropsOnBadPower(t *testing.T) {
	// If any validator's voting_power is unparseable, the aggregate metrics
	// (set_size + total_power) must not emit: a silently-skipped entry would
	// make total_power disagree with set_size.
	const badPower = `{
		"validators":[
			{"address":"g1a","voting_power":"1"},
			{"address":"g1b","voting_power":"not-a-number"}
		]
	}`
	lines := extractRPC("node-1", rpcPayload(map[string]string{"validators": badPower}))
	if len(lines) != 0 {
		t.Errorf("got %d lines; want 0 (aggregate must be all-or-nothing)", len(lines))
	}
}

func TestExtractRPC_Status_MissingValidatorInfo(t *testing.T) {
	// Some RPC responses may omit validator_info (e.g. non-validator nodes).
	const noValidatorInfo = `{"sync_info":{"latest_block_height":"10","catching_up":false}}`
	lines := extractRPC("node-1", rpcPayload(map[string]string{"status": noValidatorInfo}))
	for _, l := range lines {
		if l.Metric["__name__"] == "sentinel_rpc_validator_voting_power" {
			t.Errorf("voting_power should not be emitted when validator_info is absent")
		}
	}
	h := findLine(t, lines, map[string]string{"__name__": "sentinel_rpc_latest_block_height"})
	if h == nil || h.Values[0] != 10 {
		t.Errorf("block height still emitted, got %v", h)
	}
}
