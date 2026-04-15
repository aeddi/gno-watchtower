# Changelog

All notable changes to this project are documented here.

---

## [Unreleased] — April 2026

Testing performed on the Gno test12 testnet against a real validator infrastructure
(validator in a private network, sentry with nginx reverse proxy, existing OTel Collector as fan-out).

### Sentinel

#### Added

- **`internal/sentinel/logs/source.go`** — Non-JSON log lines are now auto-transformed instead of silently dropped
  Gnoland occasionally emits plain-text lines (e.g. during startup or after a crash) that are not valid JSON.
  Previously these lines were discarded by a `json.Valid()` guard. They are now wrapped into a valid JSON object
  `{"level":"info","msg":"<original text>"}` via the new `NormalizeLogLine` helper, so no log output is lost.
  Both `DockerSource` and `JournaldSource` use this normalisation path.

#### Fixed

- **`internal/sentinel/logs/journald_linux.go`** — `j.Wait()` return value mismatch
  `sdjournal.Journal.Wait()` returns a single `JournalWaitResult` (int), not `(JournalWaitResult, error)`.
  The two-value assignment caused a compilation failure on Linux.

- **`internal/sentinel/logs/docker.go`** — Docker log collector replayed full container history on startup
  `ContainerLogs` was called without a `Tail` option, causing the entire log history to be replayed.
  Fixed by adding `Tail: "0"`.

- **`internal/sentinel/logs/docker.go`** — Non-JSON log lines crashed the sender
  Gnoland emits plain-text lines during startup before its JSON logger is initialized.
  These were passed as `json.RawMessage` without validation, causing a marshal panic.
  Fixed by adding a `json.Valid()` guard before processing each line.

- **`internal/sentinel/logs/docker.go`** — Log collector did not reconnect after container restart
  When the validator's Docker Compose stack restarted, the sentinel lost its connection to the
  container and stopped forwarding data entirely, requiring a manual `systemctl restart sentinel`.
  Fixed by wrapping the streaming logic in a retry loop: on connection loss the collector logs a
  warning, waits 5 seconds, and reconnects automatically.

- **`internal/sentinel/metadata/collector.go`** — `telemetry.enabled` config key renamed in gnoland test12
  The key was renamed to `telemetry.metrics_enabled`. The old key produced a persistent WARN on
  every metadata collection cycle. Updated `ConfigKeys` accordingly.

- **`internal/sentinel/rpc/collector.go`** — `mempool_size` metric missing from Grafana
  `num_unconfirmed_txs` was delta-filtered after its first send (value stays `0` on idle testnet),
  resulting in a single stale data point in VictoriaMetrics invisible in Grafana's time range.
  Fixed by bypassing the delta filter for `num_unconfirmed_txs` so it is always included in each
  RPC payload.

### Watchtower

#### Fixed

- **`internal/watchtower/forwarder/forwarder.go`** — Wrong VictoriaMetrics endpoint and data format *(blocking)*
  The forwarder posted raw gnoland RPC JSON to `/api/v1/import/json`, which does not exist in
  VictoriaMetrics single-node, causing 400 errors and cascading 502s to all sentinels.
  Rewritten to extract specific numeric fields and emit proper JSON lines to `/api/v1/import`:
  - `ForwardRPC` → `peers` (from `net_info.n_peers`), `mempool_size` (from `num_unconfirmed_txs.n_txs`)
  - `ForwardMetrics` → `cpu_percent`, `memory_used_percent`, `disk_used_percent`

- **`internal/watchtower/forwarder/forwarder.go`** — OTLP handler did not decompress gzip
  The OTel Collector sends OTLP metrics with gzip compression by default. The watchtower tried to
  parse the compressed bytes as protobuf, causing `proto: cannot parse invalid wire-format data`.
  Fixed by auto-detecting gzip via magic bytes (`0x1f 0x8b`) and decompressing before protobuf parsing.

- **`deploy/Makefile`** — `make add-validator` generated empty permissions
  Double-quoting in the sed command caused `permissions = []` to be written to `watchtower.toml`,
  resulting in 403 on every sentinel endpoint. Fixed by storing the sed output in a shell variable
  to avoid the quoting conflict.

#### Documentation

- **`README.md`** — Example `watchtower.toml` used `listen_addr = "127.0.0.1:8080"`
  The watchtower was invisible to other Docker containers (including Caddy), causing 502 errors.
  Corrected to `0.0.0.0:8080`.

### Configuration Notes (not code changes)

- gnoland must be started with `--log-format=json` for the sentinel log collector to process entries
- gnoland telemetry key renamed in test12: use `telemetry.metrics_enabled` (not `telemetry.enabled`)
- OTel Collector: use `metrics_endpoint` (not `endpoint`) to avoid automatic `/v1/metrics` path appending
- OTel Collector in Docker behind a sentry: add `extra_hosts` to resolve the proxy hostname
- Loki: `retention_enabled: true` requires `delete_request_store` in the compactor config
- VictoriaMetrics healthcheck: use `127.0.0.1` instead of `localhost` to avoid IPv6 resolution issues
