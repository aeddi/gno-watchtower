# RPC vs OTLP — Signal Sourcing Guide

This document explains, for every observability signal the watchtower collects,
which gnoland pipeline we prefer (RPC poll vs OTLP push) and why. It is meant
to be consulted when adding new panels, alerts, or forwarder extractors so the
team makes consistent choices.

All claims are cited with `file:line` from the gnoland source tree. References
are against the `gnolang/gno` repository at the `master` branch; recording
sites are unchanged against the `v1.1.0` module we actually run.

## TL;DR

- **Prefer RPC for anything that has a 3-second poll answer.** Sentinel polls
  at 3s by default (`internal/sentinel/config/config.go`'s
  `rpc.poll_interval`). OTLP pushes every 60s (hardcoded in
  `tm2/pkg/telemetry/metrics/metrics.go:138` via `sdkMetric.NewPeriodicReader(exp)`
  with no period option, so the SDK default of 1 minute applies). 20x better
  freshness trumps histogram niceties.
- **Prefer OTLP for anything gnoland only exposes internally** — block build
  time, RPC server latency, VM gas/CPU, cached mempool rejections. RPC has no
  equivalent endpoint.
- **Histograms are weaker than they look.** gnoland uses the OTel SDK's
  default bucket boundaries (`[0, 10, 100, 1000, 10000, +Inf]` in whatever
  unit). At block/request rates this means every sample falls into the same
  bucket, so `histogram_quantile()` is meaningless. We extract averages via
  `rate(sum)/rate(count)` instead.
- **"Both" is a cross-check, not a choice.** Where both pipelines cover the
  same signal, we let the sentinel extract the RPC version (primary) and keep
  the OTLP version on the OTLP dashboard as a second opinion.

## Pipelines at a glance

|                        | Sentinel RPC                                             | gnoland OTLP                                       |
|------------------------|----------------------------------------------------------|----------------------------------------------------|
| Transport              | sentinel → watchtower (HTTPS POST)                       | gnoland → sentinel gRPC relay → watchtower → VM    |
| Cadence                | 3 s for status/net_info/num_unconfirmed_txs; 30 s for dump_consensus_state; on-new-block for block/block_results/validators | Event-driven `.Record()` / `.Add()` calls, aggregated in-process, flushed every **60 s** |
| Delta filter           | hash-based; `num_unconfirmed_txs` + `block` + `block_results` always-emit | none — exporter flushes all observations each period |
| Freshness at dashboard | ≤3 s for most signals                                    | up to 60 s (plus ~2 s export latency)              |
| Labels we inject       | `validator` (watchtower authenticates sentinel token)    | `validator` (watchtower adds to each ResourceMetrics); gnoland adds `service.name`, `service.instance.id`, `service.version` |

Gnoland declares 16 OTLP metrics in `tm2/pkg/telemetry/metrics/metrics.go`; the
sentinel polls 7 RPC endpoints in `internal/sentinel/rpc/collector.go`. All 16
OTLP metrics are actually recorded somewhere — verified below with `file:line`
citations — even if some only fire under load.

## Signal catalogue

Column key for **Preferred**:
- **RPC** — use the sentinel's RPC extractor; OTLP not needed.
- **OTLP** — only gnoland emits it; no RPC equivalent exists.
- **RPC primary, OTLP cross-check** — both work; we rely on RPC for dashboards,
  keep OTLP as a sanity comparison (the existing
  `test_peers_sum_matches_rpc` / `test_validator_count_matches_rpc` in
  `.ignore/tests/otlp.sh` encode this).

### Node sync & validator identity

| Signal                     | RPC source                                         | OTLP metric | Preferred | Rationale |
|----------------------------|----------------------------------------------------|-------------|-----------|-----------|
| `latest_block_height`      | `/status.sync_info.latest_block_height` (3s)       | —           | **RPC**   | No OTLP gauge; RPC is the only source. |
| `catching_up` flag         | `/status.sync_info.catching_up` (3s)               | —           | **RPC**   | Only source; critical signal. |
| Own voting power           | `/status.validator_info.voting_power` (3s)         | —           | **RPC**   | Only source. |
| Chain ID / moniker         | `/status.node_info` (unused)                       | —           | **RPC**   | Static labels; not currently extracted (see "Known gaps"). |

### Block production

| Signal                     | RPC source                              | OTLP metric                                 | Preferred                | Rationale |
|----------------------------|-----------------------------------------|---------------------------------------------|--------------------------|-----------|
| Block interval (s)         | derive from two `/status` polls         | `block_interval_hist` — `tm2/pkg/bft/consensus/state.go:1792` | **OTLP**                 | Recorded exactly at commit; deriving from RPC polls introduces 3 s jitter. |
| Block size (bytes)         | `/block` (unused)                       | `block_size_hist` — `state.go:1802`         | **OTLP**                 | Gnoland already emits; parsing full blocks is wasteful. |
| Block tx count             | `/block.header.num_txs` per-block       | `block_txs_hist` — `state.go:1800`          | **RPC primary, OTLP cross-check** | RPC updates per block (≤3 s). OTLP histogram adds nothing for num_txs specifically. |
| Build block time (ms)      | —                                       | `build_block_hist` — `state.go:1007`        | **OTLP**                 | Proposer-internal timing; no RPC equivalent. |
| Block gas price            | —                                       | `block_gas_price_hist` — `tm2/pkg/sdk/auth/keeper.go:344` | **OTLP**                 | Only source. |

### Validator set

| Signal                     | RPC source                              | OTLP metric                                  | Preferred                | Rationale |
|----------------------------|-----------------------------------------|----------------------------------------------|--------------------------|-----------|
| Active validator count     | `/validators` list length (on new block)| `validator_count_hist` — `state.go:1788`     | **RPC primary, OTLP cross-check** | RPC is fresher; OTLP acts as sanity. |
| Total voting power         | sum over `/validators`                  | `validator_vp_hist` — `state.go:1794`        | **RPC primary, OTLP cross-check** | Same reasoning. |
| Per-validator voting power | `/validators[*].voting_power` (unused)  | —                                            | **RPC**                  | Only source; not currently extracted (see "Known gaps"). |

### P2P

| Signal                     | RPC source                              | OTLP metric                     | Preferred                | Rationale |
|----------------------------|-----------------------------------------|---------------------------------|--------------------------|-----------|
| Total peer count           | `/net_info.n_peers` (3s)                | `inbound + outbound` (60s)      | **RPC**                  | 20× fresher. OTLP gauge picks up fast but is sampled at export. |
| Inbound peer count         | derive from `/net_info.peers[].is_outbound` (unused) | `inbound_peers_gauge` — `tm2/pkg/p2p/switch.go` | **OTLP**                 | gnoland already separates; parsing the RPC peer list is redundant. |
| Outbound peer count        | same                                    | `outbound_peers_gauge`          | **OTLP**                 | Same reasoning. |
| Per-peer detail (IP, id)   | `/net_info.peers[]` (unused)            | —                               | **RPC**                  | Only source; not extracted. On-demand debugging only. |

### Mempool

| Signal                     | RPC source                              | OTLP metric                                 | Preferred                | Rationale |
|----------------------------|-----------------------------------------|---------------------------------------------|--------------------------|-----------|
| Valid tx count             | `/num_unconfirmed_txs.n_txs` (always-emit, 3s) | `num_mempool_txs_hist` — `tm2/pkg/bft/mempool/clist_mempool.go:351` | **RPC primary** | RPC is `always-emit` on every poll so the gauge never goes stale at 0. OTLP only fires when mempool changes, which might be never. |
| Tx bytes                   | `/num_unconfirmed_txs.total_bytes`      | —                                           | **RPC**                  | Only source. |
| Cached/rejected tx count   | —                                       | `num_cached_txs_hist` — `clist_mempool.go:354` | **OTLP**              | Only source; fires on spam bursts. |

### Consensus internals

| Signal                     | RPC source                                   | OTLP metric | Preferred | Rationale |
|----------------------------|----------------------------------------------|-------------|-----------|-----------|
| Consensus height/round/step| `/dump_consensus_state.round_state` (30s)    | —           | **RPC**   | Only source. 30 s cadence is intentional — round_state is a big JSON blob. |
| Per-peer round state       | `/dump_consensus_state.peers[]` (unused)     | —           | **RPC**   | Only source; on-demand debugging. |

### RPC server performance

| Signal                     | RPC source | OTLP metric                                                 | Preferred | Rationale |
|----------------------------|-----------|--------------------------------------------------------------|-----------|-----------|
| HTTP request latency       | —         | `http_request_time_hist` — `tm2/pkg/bft/rpc/lib/server/handlers.go:250` | **OTLP** | Only source; intrinsic server timing. |
| WS request latency         | —         | `ws_request_time_hist` — `handlers.go:738`                   | **OTLP**  | Only source; only fires when a WS client is connected. |

### VM / contract execution

All three fire only when gnoland actually executes contract messages. An idle
devnet shows no samples — that's normal, not a regression.

| Signal                     | RPC source | OTLP metric                                                 | Preferred | Rationale |
|----------------------------|-----------|--------------------------------------------------------------|-----------|-----------|
| VM exec count              | —         | `vm_exec_msg_counter` — `gno.land/pkg/sdk/vm/keeper.go:1356` | **OTLP**  | Only source. |
| VM gas used (histogram)    | —         | `vm_gas_used_hist` — `keeper.go:1370`                        | **OTLP**  | Only source. |
| VM CPU cycles (histogram)  | —         | `vm_cpu_cycles_hist` — `keeper.go:1363`                      | **OTLP**  | Only source. |

### On-demand debugging (neither dashboard collects)

These are RPC-only and only useful interactively. The sentinel does not push
them to VictoriaMetrics.

| Signal                     | Endpoint                     | Notes |
|----------------------------|------------------------------|-------|
| Full block at height       | `/block?height=H`            | Polled for `num_txs` but the body is otherwise discarded. |
| Per-tx results             | `/block_results?height=H`    | Polled per new block (`collector.go:126`) but never extracted — candidate for a future extractor (per-tx success/failure counters, average gas, error codes). |
| Tx lookup by hash          | `/tx?hash=…`                 | Not polled. |
| Tx search                  | `/tx_search?query=…`         | Not polled. |
| ABCI app query             | `/abci_query?path=…`         | Not polled. |
| Genesis doc                | `/genesis`                   | Not polled; static per chain. |
| Consensus params           | `/consensus_params`          | Not polled; rarely changes. |

## Recording sites we verified

All OTLP metrics in `tm2/pkg/telemetry/metrics/metrics.go` are recorded in
production code paths — verified by `grep -rn "metrics.<Name>."` at the
locations cited in the tables above. No declared metric is dead; the ones that
look dead in an idle cluster (cached txs, VM metrics, gas price, WS latency)
simply depend on activity that doesn't occur on an empty testnet.

## Known gaps and future work

These are signals the sentinel could expose but currently doesn't. Each item
is small enough to do in isolation.

1. **`/block_results` extractor.** Already polled (`collector.go:126`), body
   currently discarded by the forwarder. Extract per-block
   `sentinel_rpc_block_txs_success_total`, `_failure_total`,
   `_gas_used_sum` histogram. High value when contracts start running.
2. **Chain ID and moniker labels.** `/status.node_info.network` and `.moniker`
   are static per node. Extract once at handler level and attach as labels
   to the validator's metrics — cheap, useful for multi-chain deployments.
3. **Per-validator voting power.** `/validators[*].address / .voting_power`
   is already fetched on new-block events. Emitting
   `sentinel_rpc_validator_voting_power{address=…}` would let dashboards show
   stake distribution. Cardinality is bounded by the validator set size.
4. **OTLP export period.** Gnoland hardcodes 60 s at
   `tm2/pkg/telemetry/metrics/metrics.go:138`. Making it configurable
   upstream would let us bring OTLP dashboards down to 10–15 s freshness —
   worth a PR against gnoland. Nothing we can do in watchtower alone.
5. **OTLP histogram buckets.** Default OTel SDK buckets are useless at
   gnoland scale. Configurable per-histogram buckets (e.g., block_interval
   in 0.5 s increments around 3 s) are also an upstream change.
6. **Traces.** gnoland's traces init requires `http://` or `https://`
   scheme (`tm2/pkg/telemetry/traces/traces.go:44`). The sentinel's OTLP
   relay is gRPC-only, so the cluster entrypoint hard-disables
   `telemetry.traces_enabled`. Adding an HTTP trace endpoint to the
   sentinel would unlock span collection.

## Consequences for dashboards

The OTLP dashboard (`deploy/grafana/dashboards/validators/validator-otlp.json`)
focuses on OTLP-unique signals: block build time, HTTP/WS latency, gas price
distribution, cached-tx bursts, and VM metrics. Overlapping signals (peer
count, validator set size, block tx count) live on the RPC dashboard where
they update every 3 s.

The RPC dashboard
(`deploy/grafana/dashboards/validators/validator-rpc.json`) is the primary
operational board: height, catching_up, peers, mempool, consensus state,
blocks-behind-tip.

The resources dashboards (`host-resources`, `container-resources`,
`validator-resources`) are orthogonal — they monitor the host machine and
docker container, independent of chain-level signals from either pipeline.
