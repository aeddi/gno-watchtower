# Data Collected by Watchtower

This document catalogues every observability signal the watchtower stores, the
pipeline it flows through, and — where more than one pipeline could carry it —
why we chose a specific source. Consult it before adding a panel, alert, or
extractor so the team stays consistent.

File/line citations use the `gnolang/gno` repository at `master`; the recording
sites are unchanged against the gnoland version we actually run.

## TL;DR

- **Five pipelines**: RPC poll, OTLP push, metadata collector, resource
  collector, log tail. Each is documented below with its cadence, transport,
  and delta semantics.
- **Prefer RPC for anything with a 3-second poll answer.** Sentinel polls at
  3s (`internal/sentinel/config/config.go` `rpc.poll_interval`). OTLP pushes
  every 60s (hardcoded in `tm2/pkg/telemetry/metrics/metrics.go:138` via
  `sdkMetric.NewPeriodicReader(exp)` with no period option — SDK default 1m
  applies). 20× better freshness trumps histogram niceties.
- **Prefer OTLP for gnoland internals.** Block build time, RPC server latency,
  VM gas/CPU, cached mempool rejections, block gas price — no RPC equivalent.
- **Where both sides cover the same signal, the sentinel drops the OTLP version
  before forwarding** (`internal/sentinel/otlp/filter.go` — the
  `deniedMetricNames` deny-list). So in practice "both" never shows up in VM;
  see the filter section below.
- **OTLP histograms are weaker than they look.** Default OTel SDK buckets
  `[0, 10, 100, 1000, 10000, +Inf]` collapse all samples into one bucket at
  gnoland scale, so `histogram_quantile()` is meaningless. We extract averages
  via `rate(sum)/rate(count)` instead.

## Pipelines at a glance

|                 | RPC (poll + one-shot)                                                                                                      | OTLP                                                                                                | Metadata                                   | Resources                        | Logs                              |
| --------------- | -------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------- | ------------------------------------------ | -------------------------------- | --------------------------------- |
| Source          | sentinel polls gnoland RPC                                                                                                 | gnoland → sentinel HTTP relay on :4318                                                              | sentinel reads gnoland config.toml         | sentinel gopsutil + docker stats | sentinel tails gnoland stdout     |
| Backend         | VictoriaMetrics                                                                                                            | VictoriaMetrics (OTel endpoint)                                                                     | VictoriaMetrics                            | VictoriaMetrics                  | Loki                              |
| Transport       | sentinel → watchtower HTTPS                                                                                                | sentinel → watchtower HTTPS (protobuf)                                                              | sentinel → watchtower HTTPS                | sentinel → watchtower HTTPS      | sentinel → watchtower HTTPS       |
| Cadence         | 3s (status/net_info/num_unconfirmed_txs); 30s (dump_consensus_state); on new block (block, validators); one-shot (genesis) | Per-event `.Record()`/`.Add()`, flushed every 60s                                                   | 10-min timer + fsnotify on config change   | 10s                              | streaming                         |
| Delta filter    | hash-based; `num_unconfirmed_txs` + `block` always-emit                                                                    | none — exporter flushes all observations each period; sentinel relay drops denied names (see below) | hash-based — emits only when values change | hash-based per-key               | none (every line forwarded)       |
| Validator label | watchtower-injected from authenticated token                                                                               | watchtower-injected as a ResourceMetrics attribute                                                  | watchtower-injected                        | watchtower-injected              | watchtower-injected as Loki label |

Gnoland declares 16 OTLP metrics in `tm2/pkg/telemetry/metrics/metrics.go`; the
sentinel polls 6 RPC endpoints in `internal/sentinel/rpc/collector.go` plus
`/genesis` once on startup. All 16 OTLP metrics are recorded in production code
paths — verified with `grep -rn "metrics.<Name>."` at the locations cited
below.

### Optional beacon hop (sentry-fronted validators)

When the validator sits behind a sentry, the sentinel connects to a **beacon**
on the sentry over Noise-encrypted TCP (`pkg/noise`); the beacon forwards all
five pipelines to the watchtower over HTTPS, unchanged and without re-
authenticating (the sentinel's bearer token flows through untouched). One
exception: the RPC pipeline's payload is augmented in-flight — the beacon
fetches the sentry's own `/status`, `/net_info`, and config keys and injects
them as `sentry_status`, `sentry_net_info`, `sentry_config` alongside the
validator-side keys (`internal/beacon/augment/augment.go`). These surface as
the `sentinel_sentry_*` metric family — see
[Sentry identity & view](#sentry-identity--view-optional-beacon-only).

Auth is unchanged: the watchtower still authenticates by bearer token, so a
compromised beacon cannot impersonate the validator. The beacon's own Noise
identity is optionally pinned on both sides (sentinel pins the beacon's
pubkey; beacon allowlists accepted sentinel pubkeys) — see README.

## Signal catalogue

Column key for **Preferred**:

- **RPC** — sentinel's RPC extractor; OTLP equivalent (if any) is denied at the
  sentinel filter.
- **OTLP** — only gnoland emits it; no RPC equivalent.

### Node sync & validator identity

| Signal                       | RPC source                                      | Metric emitted                                                 | Preferred | Rationale                                                 |
| ---------------------------- | ----------------------------------------------- | -------------------------------------------------------------- | --------- | --------------------------------------------------------- |
| `latest_block_height`        | `/status.sync_info.latest_block_height` (3s)    | `sentinel_rpc_latest_block_height`                             | **RPC**   | No OTLP gauge.                                            |
| `catching_up` flag           | `/status.sync_info.catching_up` (3s)            | `sentinel_rpc_catching_up`                                     | **RPC**   | Only source; critical signal.                             |
| Own voting power             | `/status.validator_info.voting_power` (3s)      | `sentinel_rpc_validator_voting_power`                          | **RPC**   | Only source.                                              |
| Chain ID / moniker / version | `/status.node_info.{network, moniker, version}` | `sentinel_node_build_info{chain_id,moniker,version}` (value=1) | **RPC**   | Emitted as a Prometheus info-metric on every status poll. |

### Sentry identity & view (optional, beacon-only)

Signals injected by the beacon's `/rpc` augmenter
(`internal/beacon/augment/augment.go`) on every tick that carries
`net_info`. Present only when a beacon fronts the validator; absent
otherwise. All three carry the validator label like the rest of the
catalogue.

| Signal             | Source                                                         | Metric emitted                                                               | Rationale                                                                                                               |
| ------------------ | -------------------------------------------------------------- | ---------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| Sentry build info  | beacon fetches sentry's `/status.node_info` per RPC tick       | `sentinel_sentry_info{sentry_chain,sentry_moniker,sentry_version}` (value=1) | Lets metadata dashboards show the sentry's version alongside the validator's — drift between the two is a deploy smell. |
| Sentry peer count  | beacon fetches sentry's `/net_info.n_peers` per RPC tick       | `sentinel_rpc_peers_via_sentry`                                              | On sentry-fronted setups the validator sees only the sentry; the real p2p surface is what the sentry reports.           |
| Sentry config keys | beacon reads sentry's config (same `ConfigKeys` as `metadata`) | `sentinel_sentry_config{key,value}` (value=1)                                | Pair-wise drift check: compare `sentinel_node_config` against `sentinel_sentry_config` for the same validator.          |

The beacon fails open: any fetch error logs a warning and forwards the
original payload unchanged, so `sentinel_sentry_*` series simply go stale
until the next successful augmentation.

### Block / genesis / config metadata

These are Prometheus info-style metrics (`value = 1`, identifying data lives in
labels). The metadata dashboard pivots them to spot fleet drift.

| Signal                   | Source                                            | Metric emitted                                                         | Rationale                                                                                                                                                                                                          |
| ------------------------ | ------------------------------------------------- | ---------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Binary build info        | `/status.node_info` (3s)                          | `sentinel_node_build_info{chain_id,moniker,version}`                   | gnoland exposes the version directly once `-ldflags -X tm2/pkg/version.Version=…` is baked into the binary.                                                                                                        |
| Genesis identity         | `/genesis` (one-shot at sentinel startup)         | `sentinel_genesis_info{chain_id,genesis_time,app_hash}`                | `/genesis` is static per chain. The sentinel fetches once, sets a `genesisSent` flag, and never refetches.                                                                                                         |
| Genesis consensus params | `/genesis.consensus_params`                       | `sentinel_genesis_consensus_param{param,value}` (one series per param) | 6 params: block.max\_{tx,data,block}\_bytes, block.max_gas, block.time_iota_ms, validator.pub_key_types.                                                                                                           |
| Genesis validator set    | `/genesis.validators[]`                           | `sentinel_genesis_validator{address,pub_key,power,name}`               | One series per genesis validator per reporter. Fleet disagreement here implies a genesis fork.                                                                                                                     |
| gnoland config keys      | sentinel metadata collector (fsnotify + 10m tick) | `sentinel_node_config{key,value}`                                      | 7 keys: `application.prune_strategy`, `consensus.{peer_gossip_sleep_duration,timeout_commit}`, `mempool.size`, `p2p.{flush_throttle_timeout,max_num_outbound_peers,pex}`. Every key is one RPC/OTLP cannot expose. |

### Block production

| Signal                | RPC source                        | OTLP metric                                                   | Preferred | Rationale                                                          |
| --------------------- | --------------------------------- | ------------------------------------------------------------- | --------- | ------------------------------------------------------------------ |
| Block interval (s)    | —                                 | `block_interval_hist` — `tm2/pkg/bft/consensus/state.go:1792` | **OTLP**  | Recorded exactly at commit; no RPC equivalent.                     |
| Block size (bytes)    | —                                 | `block_size_hist` — `state.go:1802`                           | **OTLP**  | Only source after the sentinel stopped parsing full /block bodies. |
| Block tx count        | `/block.header.num_txs` per-block | `block_txs_hist` **denied**                                   | **RPC**   | Sentinel filter drops `block_txs_hist` — RPC at 3s is fresher.     |
| Build block time (ms) | —                                 | `build_block_hist` — `state.go:1007`                          | **OTLP**  | Proposer-internal timing.                                          |
| Block gas price       | —                                 | `block_gas_price_hist` — `tm2/pkg/sdk/auth/keeper.go:344`     | **OTLP**  | Only source.                                                       |

### Validator set

| Signal                     | RPC source                                             | OTLP metric                       | Preferred | Rationale                                                                                                                                                                                                                                                                                         |
| -------------------------- | ------------------------------------------------------ | --------------------------------- | --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Active validator count     | `/validators` list length (on new block)               | `validator_count_hist` **denied** | **RPC**   | Denied at sentinel — RPC is a fresher gauge, simpler to query.                                                                                                                                                                                                                                    |
| Total voting power         | sum over `/validators`                                 | `validator_vp_hist` **denied**    | **RPC**   | Same.                                                                                                                                                                                                                                                                                             |
| Per-validator voting power | `/validators[*].{address,voting_power}` (on new block) | —                                 | **RPC**   | `sentinel_rpc_validator_set_power{address,validator}` — one series per (validator-in-set × reporter). Dashboards dedupe across reporters via `max by (address) (...)` and join on `sentinel_validator_online{address}` to compute active voting power. Cardinality bounded by validator-set size. |

### P2P

| Signal                   | RPC source                                           | OTLP metric                                     | Preferred | Rationale                                                                         |
| ------------------------ | ---------------------------------------------------- | ----------------------------------------------- | --------- | --------------------------------------------------------------------------------- |
| Total peer count         | `/net_info.n_peers` (3s)                             | `inbound + outbound`                            | **RPC**   | 20× fresher.                                                                      |
| Inbound peer count       | derive from `/net_info.peers[].is_outbound` (unused) | `inbound_peers_gauge` — `tm2/pkg/p2p/switch.go` | **OTLP**  | Gnoland splits inbound/outbound natively; parsing the RPC peer list is redundant. |
| Outbound peer count      | same                                                 | `outbound_peers_gauge`                          | **OTLP**  | Same.                                                                             |
| Per-peer detail (IP, id) | `/net_info.peers[]` (unused)                         | —                                               | —         | On-demand debugging only; not pushed to VM.                                       |

### Mempool

| Signal                   | RPC source                                     | OTLP metric                                    | Preferred | Rationale                                                             |
| ------------------------ | ---------------------------------------------- | ---------------------------------------------- | --------- | --------------------------------------------------------------------- |
| Valid tx count           | `/num_unconfirmed_txs.n_txs` (always-emit, 3s) | `num_mempool_txs_hist` **denied**              | **RPC**   | RPC is `always-emit` so the gauge never goes stale at 0. OTLP denied. |
| Tx bytes                 | `/num_unconfirmed_txs.total_bytes`             | —                                              | **RPC**   | Only source.                                                          |
| Cached/rejected tx count | —                                              | `num_cached_txs_hist` — `clist_mempool.go:354` | **OTLP**  | Only source; fires on spam bursts.                                    |

### Consensus internals

| Signal                      | RPC source                                | OTLP metric | Preferred |
| --------------------------- | ----------------------------------------- | ----------- | --------- |
| Consensus height/round/step | `/dump_consensus_state.round_state` (30s) | —           | **RPC**   |
| Per-peer round state        | `/dump_consensus_state.peers[]` (unused)  | —           | —         |

### RPC server performance

| Signal               | OTLP metric                                                             | Preferred | Rationale                                 |
| -------------------- | ----------------------------------------------------------------------- | --------- | ----------------------------------------- |
| HTTP request latency | `http_request_time_hist` — `tm2/pkg/bft/rpc/lib/server/handlers.go:250` | **OTLP**  | Only source.                              |
| WS request latency   | `ws_request_time_hist` — `handlers.go:738`                              | **OTLP**  | Only fires when a WS client is connected. |

### VM / contract execution

All three fire only when gnoland actually executes contract messages. An idle
devnet shows no samples — that's normal.

| Signal        | OTLP metric                                                  |
| ------------- | ------------------------------------------------------------ |
| VM exec count | `vm_exec_msg_counter` — `gno.land/pkg/sdk/vm/keeper.go:1356` |
| VM gas used   | `vm_gas_used_hist` — `keeper.go:1370`                        |
| VM CPU cycles | `vm_cpu_cycles_hist` — `keeper.go:1363`                      |

### Resources

Sentinel emits host + container gauges from gopsutil (host) and the docker
stats API (container). The forwarder turns them into Prometheus samples keyed
by `validator`.

| Signal                     | Source                               | Metric                                                            |
| -------------------------- | ------------------------------------ | ----------------------------------------------------------------- |
| Host CPU %                 | gopsutil `cpu.PercentWithContext`    | `sentinel_host_cpu_percent`                                       |
| Host memory (total/used/…) | gopsutil `mem.VirtualMemory`         | `sentinel_host_memory_{total,available,used,free}_bytes`          |
| Host disk (root fs)        | gopsutil `disk.Usage("/")`           | `sentinel_host_disk_{total,free,used}_bytes{path,fstype}`         |
| Host network               | gopsutil `net.IOCounters(false)`     | `sentinel_host_network_{receive,transmit}_bytes_total{interface}` |
| Container CPU              | `docker stats` cpu_usage.total_usage | `sentinel_container_cpu_usage_seconds_total{container}`           |
| Container memory           | `docker stats` memory_stats          | `sentinel_container_memory_{usage,limit,working_set}_bytes`       |
| Container network          | `docker stats` networks[]            | `sentinel_container_network_{receive,transmit}_bytes_total`       |

See `internal/sentinel/resources/collector.go` for the poll implementation and
`internal/watchtower/forwarder/metrics.go` for the extractors.

### Logs

gnoland's stdout (zap JSON by default) → sentinel log collector (docker or
journald) → watchtower → Loki.

| Signal              | Source                                             | Loki shape                                                                  |
| ------------------- | -------------------------------------------------- | --------------------------------------------------------------------------- |
| Validator log lines | sentinel log collector (`internal/sentinel/logs/`) | indexed labels: `{validator, level, module}`; body is the raw JSON log line |

The sentinel guarantees every line has `ts`/`level`/`msg`/`module` populated
(`internal/sentinel/logs/source.go` — `EnsureJSON`). Non-JSON stdout is wrapped
as `module="sentinel-raw"` so it remains queryable. The forwarder splits
payloads by module so `module` becomes an indexed Loki label (supports
`label_values(module)` in Grafana dropdowns).

## Sentinel OTLP deny-list

Before forwarding each `ExportMetricsServiceRequest`, the sentinel drops
metrics that the RPC pipeline provides a fresher equivalent for. Source:
`internal/sentinel/otlp/filter.go`, `deniedMetricNames`.

| Denied OTLP metric     | Superseded by                                    |
| ---------------------- | ------------------------------------------------ |
| `block_txs_hist`       | `sentinel_rpc_block_num_txs` (per-block, RPC)    |
| `validator_count_hist` | `sentinel_rpc_validator_set_size` (on new block) |
| `validator_vp_hist`    | `sentinel_rpc_validator_set_total_power`         |
| `num_mempool_txs_hist` | `sentinel_rpc_mempool_txs` (always-emit, 3s)     |

Filtering at the sentinel (rather than watchtower) saves bandwidth on the
sentinel→watchtower link and prevents the two pipelines from fighting for the
same dashboard slot.

## On-demand debugging (not pushed to VM)

| Endpoint                  | Notes                                                                                 |
| ------------------------- | ------------------------------------------------------------------------------------- |
| `/block?height=H`         | Polled at each new block — only `num_txs` is extracted, body otherwise discarded.     |
| `/block_results?height=H` | No longer polled (`refactor(sentinel): stop polling unused /block_results endpoint`). |
| `/tx?hash=…`              | Not polled.                                                                           |
| `/tx_search?query=…`      | Not polled.                                                                           |
| `/abci_query?path=…`      | Not polled.                                                                           |
| `/consensus_params`       | Covered indirectly by `sentinel_genesis_consensus_param`.                             |

## Known gaps and future work

1. **OTLP export period.** Gnoland hardcodes 60s at
   `tm2/pkg/telemetry/metrics/metrics.go:138`. Making it configurable upstream
   would drop OTLP dashboard staleness to 10–15s. Nothing watchtower can do
   alone.
2. **OTLP histogram buckets.** Default OTel SDK buckets are useless at gnoland
   scale. Per-histogram custom buckets are an upstream change.
3. **Traces — accepted, not forwarded yet.** gnoland can emit OTel spans
   around every RPC handler call (`tm2/pkg/bft/rpc/core/*.go` — each handler
   wraps its body in a `traces.Tracer().Start(…)` span with `http.method`,
   `http.path`, `remoteAddr` attributes on the root). The sentinel's relay
   accepts `POST /v1/traces` so turning on `telemetry.traces_enabled` is
   crash-safe, but the bytes are discarded immediately — we have no trace
   backend.

    **What adding a backend would unlock.** Primarily: per-request RPC drill-
    down ("which caller is slamming /status at 10 req/s?" — aggregated
    `http_request_time_hist` hides the `remoteAddr`). Individual slow-request
    inspection instead of just the p99. A "service map" panel showing
    callers → RPC server.

    **What it wouldn't unlock.** Block production internals, consensus state
    transitions, mempool admission, VM execution, P2P — gnoland emits no
    spans for any of these today. The interesting chain-internal work is
    already covered by the OTLP histograms (`build_block_hist`,
    `block_interval_hist`, `vm_*_hist`).

    **Cost to add.** Tempo (or equivalent) in the stack (~500 MB container
    - object-store retention), a watchtower trace forwarder, a new Grafana
      datasource, one dashboard. Gnoland doesn't need to change — the HTTP
      relay is already spec-compliant.

    **Why defer.** The one unique operator signal (who's calling the RPC
    server) isn't worth a new storage backend today, and most chain-internal
    observability is already handled elsewhere. When gnoland starts spanning
    consensus/VM paths — or when we need external-caller auditing — this
    flips; the relay side is already in place, so it's a pure stack
    addition.

## Consequences for dashboards

`deploy/grafana/dashboards/` hosts six dashboards across two subdirectories:

`validators/` — per-validator operational boards:

- **`validator-chain.json`** — primary operational board. Merges RPC and OTLP
  signals: height, catching_up, peers, mempool, consensus state,
  blocks-behind-tip, block-build time, RPC server latency, gas price, plus a
  fleet-wide consensus-quorum row. When a beacon is deployed, the RPC snapshot
  gains `sentry` / `peers_via_sentry` / `pex` / `pex_sentry` columns and the
  Peers panel overlays direct vs. via-sentry series.
- **`validator-metadata.json`** — fleet drift board. Build info, genesis
  (consensus params + validator set), config keys. Consistency stats flag
  divergence as red cells. When a beacon is deployed, Build info adds
  `sentry_moniker` + `sentry_version` columns and a separate "Config values
  (sentry)" table surfaces the sentry-side config drift.
- **`validator-resources.json`** — per-validator host + container resources
  (sentinel-sourced — the only pipeline that can see remote validators' hosts).
- **`node-logs.json`** — Loki log viewer with module/level filters.

`watchtower/` — watchtower-host-side infrastructure boards:

- **`host-resources.json`** — watchtower host stats via node-exporter.
- **`container-resources.json`** — containers on the watchtower host via
  cAdvisor. Useful in the dev cluster where validators are co-located; in
  production only covers the watchtower's own containers.
