package forwarder

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/aeddi/gno-watchtower/pkg/gpub"
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
	netInfoJSON   = `{"n_peers":"3"}`
	mempoolJSON   = `{"n_txs":"2","total":"2","total_bytes":"512"}`
	consensusJSON = `{"round_state":{"height":"4478","round":0,"step":1}}`
	// 32-byte ed25519 payloads chosen so the expected gpub bech32 strings can
	// be computed deterministically by pkg/gpub in the test — see
	// TestExtractRPC_Validators_PubKeyBech32Label. Real /validators responses
	// carry the same shape: pub_key.{@type,value} with the raw key base64-encoded.
	validatorsJSON = `{
		"block_height":"4478",
		"validators":[
			{"address":"g1a","pub_key":{"@type":"/tm.PubKeyEd25519","value":"mKsg1XPxANeixURll0tm+FdymdT7qyOMs8h0lliCK6w="},"voting_power":"1"},
			{"address":"g1b","pub_key":{"@type":"/tm.PubKeyEd25519","value":"FPseTktiYHImHBZ1wC0Gv9ifZ9hASDjUlKqNDh2jS+I="},"voting_power":"2"},
			{"address":"g1c","pub_key":{"@type":"/tm.PubKeyEd25519","value":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="},"voting_power":"5"}
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
		"sentinel_node_build_info",
		"sentinel_rpc_catching_up",
		"sentinel_rpc_latest_block_height",
		"sentinel_rpc_validator_voting_power",
		"sentinel_validator_online",
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
	// 1 set_size + 1 set_total_power + 3 set_power (one per validator) = 5.
	want := []string{
		"sentinel_rpc_validator_set_power",
		"sentinel_rpc_validator_set_power",
		"sentinel_rpc_validator_set_power",
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
	// Per-member set_power: check g1b has power=2.
	pb := findLine(t, lines, map[string]string{"__name__": "sentinel_rpc_validator_set_power", "address": "g1b"})
	if pb == nil || pb.Values[0] != 2 {
		t.Errorf("set_power{address=g1b} = %v, want 2", pb)
	}
	if pb != nil && pb.Metric["validator"] != "node-1" {
		t.Errorf("set_power reporter label = %q, want node-1", pb.Metric["validator"])
	}
}

// TestExtractRPC_Validators_PubKeyBech32Label asserts that each set_power
// series carries pub_key_bech32 with the canonical `gpub1...` encoding of
// the validator's pub_key.value — what `gnoland secrets get
// validator_key.pub_key -raw` would print for the same key. The dashboard's
// voting-power panel surfaces this as the identity column; the encoding
// itself is pinned by pkg/gpub so this test only checks the integration.
func TestExtractRPC_Validators_PubKeyBech32Label(t *testing.T) {
	lines := extractRPC("node-1", rpcPayload(map[string]string{"validators": validatorsJSON}))
	// g1b: pub_key value matches node-1 from cluster fixture.
	wantB, err := gpub.EncodeEd25519FromBase64("FPseTktiYHImHBZ1wC0Gv9ifZ9hASDjUlKqNDh2jS+I=")
	if err != nil {
		t.Fatal(err)
	}
	pb := findLine(t, lines, map[string]string{"__name__": "sentinel_rpc_validator_set_power", "address": "g1b"})
	if pb == nil {
		t.Fatal("set_power{address=g1b} missing")
	}
	if pb.Metric["pub_key_bech32"] != wantB {
		t.Errorf("set_power{g1b}.pub_key_bech32 = %q, want %q", pb.Metric["pub_key_bech32"], wantB)
	}
	// Every set_power line should carry pub_key_bech32 non-empty (fixture
	// gives pub_key on all entries).
	for _, l := range lines {
		if l.Metric["__name__"] != "sentinel_rpc_validator_set_power" {
			continue
		}
		if l.Metric["pub_key_bech32"] == "" {
			t.Errorf("set_power{address=%s}: pub_key_bech32 label missing", l.Metric["address"])
		}
	}
}

// TestExtractRPC_Validators_PubKeyBech32Omitted_WhenMissing covers the
// forward-compat case: if a gnoland /validators response drops pub_key (or
// the type isn't ed25519), the set_power line is still emitted without the
// pub_key_bech32 label — we never fabricate an empty-string label.
func TestExtractRPC_Validators_PubKeyBech32Omitted_WhenMissing(t *testing.T) {
	const noPubKey = `{
		"validators":[
			{"address":"g1x","voting_power":"1"}
		]
	}`
	lines := extractRPC("node-1", rpcPayload(map[string]string{"validators": noPubKey}))
	p := findLine(t, lines, map[string]string{"__name__": "sentinel_rpc_validator_set_power", "address": "g1x"})
	if p == nil {
		t.Fatal("set_power{address=g1x} missing — aggregate should still emit")
	}
	if _, has := p.Metric["pub_key_bech32"]; has {
		t.Errorf("pub_key_bech32 label should be absent when pub_key missing; got %v", p.Metric)
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
	// status 5 (height + catching_up + voting_power + build_info + online) +
	// net_info 1 + mempool 2 + consensus 3 + validators 5 (size + total + 3× set_power) +
	// block 1 = 17.
	if len(lines) != 17 {
		t.Errorf("got %d lines, want 17", len(lines))
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

func TestExtractRPC_Status_OnlineEmittedWhenCaughtUp(t *testing.T) {
	// When catching_up=false AND validator_info.address is present, the
	// forwarder emits sentinel_validator_online{validator,address} 1 — the
	// dashboard's "who's live right now" presence signal. The address label
	// feeds consensus-quorum joins (on address); the validator label feeds
	// per-validator panels that key by reporter.
	lines := extractRPC("node-1", rpcPayload(map[string]string{"status": statusJSON}))
	online := findLine(t, lines, map[string]string{"__name__": "sentinel_validator_online", "address": "g1self"})
	if online == nil {
		t.Fatal("sentinel_validator_online not emitted")
	}
	if online.Values[0] != 1 {
		t.Errorf("online value = %v, want 1", online.Values[0])
	}
	if online.Metric["validator"] != "node-1" {
		t.Errorf("online.validator = %q, want node-1 (needed for per-validator joins)", online.Metric["validator"])
	}
}

func TestExtractRPC_Status_OnlineNotEmittedWhenCatchingUp(t *testing.T) {
	lines := extractRPC("node-1", rpcPayload(map[string]string{"status": statusCatchingUpJSON}))
	for _, l := range lines {
		if l.Metric["__name__"] == "sentinel_validator_online" {
			t.Fatalf("sentinel_validator_online must NOT be emitted while catching up (got %v)", l.Metric)
		}
	}
}

func TestExtractRPC_Status_VotingPowerCarriesAddress(t *testing.T) {
	// The voting_power metric's address label lets consensus-quorum dashboards
	// join against sentinel_rpc_validator_set_power via on(address).
	lines := extractRPC("node-1", rpcPayload(map[string]string{"status": statusJSON}))
	vp := findLine(t, lines, map[string]string{"__name__": "sentinel_rpc_validator_voting_power", "address": "g1self"})
	if vp == nil {
		t.Fatal("voting_power with address label not emitted")
	}
	if vp.Metric["validator"] != "node-1" {
		t.Errorf("voting_power.validator = %q, want node-1", vp.Metric["validator"])
	}
}

func TestExtractRPC_Status_BuildInfoCarriesChainAndVersion(t *testing.T) {
	const statusWithNode = `{
		"node_info":{"moniker":"mynode","network":"test-chain","version":"master.12345+abcdef0"},
		"sync_info":{"latest_block_height":"1","catching_up":false},
		"validator_info":{"voting_power":"1"}
	}`
	lines := extractRPC("node-1", rpcPayload(map[string]string{"status": statusWithNode}))
	info := findLine(t, lines, map[string]string{"__name__": "sentinel_node_build_info"})
	if info == nil {
		t.Fatal("sentinel_node_build_info not emitted")
	}
	if info.Metric["chain_id"] != "test-chain" {
		t.Errorf("chain_id = %q, want test-chain", info.Metric["chain_id"])
	}
	if info.Metric["version"] != "master.12345+abcdef0" {
		t.Errorf("version = %q", info.Metric["version"])
	}
	if info.Metric["moniker"] != "mynode" {
		t.Errorf("moniker = %q", info.Metric["moniker"])
	}
	if info.Values[0] != 1 {
		t.Errorf("info value = %v, want 1", info.Values[0])
	}
}

const genesisJSON = `{
	"genesis":{
		"genesis_time":"2026-01-01T00:00:00Z",
		"chain_id":"test-chain",
		"app_hash":"deadbeef",
		"consensus_params":{
			"Block":{"MaxTxBytes":"1000000","MaxDataBytes":"2000000","MaxBlockBytes":"0","MaxGas":"3000000000","TimeIotaMS":"100"},
			"Validator":{"PubKeyTypeURLs":["/tm.PubKeyEd25519","/tm.PubKeySecp256k1"]}
		},
		"validators":[
			{"address":"g1abc","pub_key":{"value":"vkA"},"power":"1","name":"node-1"},
			{"address":"g1def","pub_key":{"value":"vkB"},"power":"2","name":"node-2"}
		]
	}
}`

func TestExtractRPC_Genesis(t *testing.T) {
	lines := extractRPC("node-1", rpcPayload(map[string]string{"genesis": genesisJSON}))
	// 1 genesis_info + 6 consensus_params + 2 validators = 9 lines.
	if len(lines) != 9 {
		t.Fatalf("got %d lines, want 9", len(lines))
	}
	info := findLine(t, lines, map[string]string{"__name__": "sentinel_genesis_info"})
	if info == nil || info.Metric["chain_id"] != "test-chain" {
		t.Errorf("genesis_info chain_id not carried: %v", info)
	}
	if info != nil && info.Metric["app_hash"] != "deadbeef" {
		t.Errorf("app_hash = %q, want deadbeef", info.Metric["app_hash"])
	}
	// consensus_params: expect block.max_tx_bytes=1000000.
	maxTx := findLine(t, lines, map[string]string{"__name__": "sentinel_genesis_consensus_param", "param": "block.max_tx_bytes"})
	if maxTx == nil || maxTx.Metric["value"] != "1000000" {
		t.Errorf("max_tx_bytes param missing or wrong: %v", maxTx)
	}
	// validator pub key types joined with comma.
	pkt := findLine(t, lines, map[string]string{"__name__": "sentinel_genesis_consensus_param", "param": "validator.pub_key_types"})
	if pkt == nil || pkt.Metric["value"] != "/tm.PubKeyEd25519,/tm.PubKeySecp256k1" {
		t.Errorf("pub_key_types join failed: %v", pkt)
	}
	// Each genesis validator gets a series.
	var gotValidators int
	for _, l := range lines {
		if l.Metric["__name__"] == "sentinel_genesis_validator" {
			gotValidators++
		}
	}
	if gotValidators != 2 {
		t.Errorf("sentinel_genesis_validator count = %d, want 2", gotValidators)
	}
}

func TestExtractRPC_UnknownKey_Ignored(t *testing.T) {
	// "block_results" is a deliberate canary: a Tendermint RPC endpoint the
	// sentinel does NOT poll. If this test ever starts producing lines, the
	// switch in extractRPC has grown a case it shouldn't have.
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

// ---- sentry_* extractors (beacon-injected keys)

const sentryStatusJSON = `{
	"node_info":{"moniker":"sentry-a","network":"test-chain","version":"master.99999+feedbee"},
	"sync_info":{"latest_block_height":"4500","catching_up":false}
}`

const sentryNetInfoJSON = `{"n_peers":"25"}`

const sentryConfigJSON = `{"p2p.pex":"true","p2p.max_num_outbound_peers":"20"}`

func TestExtractRPC_SentryStatus(t *testing.T) {
	lines := extractRPC("node-1", rpcPayload(map[string]string{"sentry_status": sentryStatusJSON}))
	info := findLine(t, lines, map[string]string{"__name__": "sentinel_sentry_info"})
	if info == nil {
		t.Fatal("sentinel_sentry_info not emitted")
	}
	if info.Metric["validator"] != "node-1" {
		t.Errorf("validator label = %q, want node-1", info.Metric["validator"])
	}
	if info.Metric["sentry_moniker"] != "sentry-a" {
		t.Errorf("sentry_moniker = %q, want sentry-a", info.Metric["sentry_moniker"])
	}
	if info.Metric["sentry_chain"] != "test-chain" {
		t.Errorf("sentry_chain = %q", info.Metric["sentry_chain"])
	}
	if info.Metric["sentry_version"] != "master.99999+feedbee" {
		t.Errorf("sentry_version = %q", info.Metric["sentry_version"])
	}
	if info.Values[0] != 1 {
		t.Errorf("info value = %v, want 1", info.Values[0])
	}
}

func TestExtractRPC_SentryNetInfo(t *testing.T) {
	lines := extractRPC("node-1", rpcPayload(map[string]string{"sentry_net_info": sentryNetInfoJSON}))
	if got := metricNames(t, lines); !slices.Equal(got, []string{"sentinel_rpc_peers_via_sentry"}) {
		t.Fatalf("sentry_net_info metric names = %v", got)
	}
	p := findLine(t, lines, map[string]string{"__name__": "sentinel_rpc_peers_via_sentry"})
	if p == nil || p.Values[0] != 25 {
		t.Errorf("peers_via_sentry = %v, want 25", p)
	}
	if p != nil && p.Metric["validator"] != "node-1" {
		t.Errorf("peers_via_sentry.validator = %q", p.Metric["validator"])
	}
}

func TestExtractRPC_SentryConfig(t *testing.T) {
	lines := extractRPC("node-1", rpcPayload(map[string]string{"sentry_config": sentryConfigJSON}))
	if len(lines) != 2 {
		t.Fatalf("sentry_config emitted %d lines, want 2", len(lines))
	}
	pex := findLine(t, lines, map[string]string{"__name__": "sentinel_sentry_config", "key": "p2p.pex"})
	if pex == nil {
		t.Fatal("sentry_config{key=p2p.pex} not found")
	}
	if pex.Metric["value"] != "true" {
		t.Errorf("p2p.pex value = %q, want true", pex.Metric["value"])
	}
	if pex.Metric["validator"] != "node-1" {
		t.Errorf("sentry_config.validator = %q", pex.Metric["validator"])
	}
}

func TestExtractRPC_SentryConfig_MalformedSkipped(t *testing.T) {
	lines := extractRPC("node-1", rpcPayload(map[string]string{"sentry_config": `not-json`}))
	if len(lines) != 0 {
		t.Errorf("malformed sentry_config produced %d lines, want 0", len(lines))
	}
}

func TestExtractRPC_Reachable_EmitsGauge(t *testing.T) {
	lines := extractRPC("node-1", rpcPayload(map[string]string{"rpc_reachable": `1`}))
	if got := metricNames(t, lines); len(got) != 1 || got[0] != "sentinel_rpc_reachable" {
		t.Fatalf("metric names = %v, want [sentinel_rpc_reachable]", got)
	}
	line := lines[0]
	if line.Values[0] != 1 {
		t.Errorf("value = %v, want 1 (reachable)", line.Values[0])
	}
	if line.Metric["validator"] != "node-1" {
		t.Errorf("validator = %q, want node-1", line.Metric["validator"])
	}
}

func TestExtractRPC_Reachable_ZeroWhenUnreachable(t *testing.T) {
	lines := extractRPC("node-1", rpcPayload(map[string]string{"rpc_reachable": `0`}))
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}
	if lines[0].Values[0] != 0 {
		t.Errorf("value = %v, want 0 (unreachable)", lines[0].Values[0])
	}
}

func TestExtractRPC_Reachable_MalformedDroppedSilently(t *testing.T) {
	lines := extractRPC("node-1", rpcPayload(map[string]string{"rpc_reachable": `"nope"`}))
	if len(lines) != 0 {
		t.Errorf("malformed rpc_reachable produced %d lines, want 0", len(lines))
	}
}
