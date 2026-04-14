# gno-watchtower — Testing Feedback

Testing performed on April 13, 2026 against the Gno test12 testnet.

---

## Test Infrastructure

The stack was tested against a real validator setup, not a local environment:

- **Validator node** — runs gnoland inside Docker, isolated in a private network with no direct internet access
- **Sentry node** — acts as the P2P relay for the validator; also hosts an nginx reverse proxy to bridge the sentinel to the watchtower (the sentinel cannot reach the watchtower directly)
- **Monitoring server** — runs the watchtower stack (watchtower + VictoriaMetrics + Loki + Grafana + Caddy) on a public-facing machine
- **OTel Collector** — an existing OpenTelemetry Collector instance already present on the validator was reused as a fan-out: it receives gnoland's OTLP metrics and forwards them both to a local Prometheus endpoint and to the watchtower

The sentinel's built-in OTLP relay (`[otlp]` section) was therefore **not used** — the existing OTel Collector covers that role and allows dual export.

The nginx reverse proxy on the sentry injects the real Bearer token, so the sentinel config uses a dummy token value:

```nginx
location /watchtower/ {
    allow <validator-ip>;
    deny all;

    proxy_pass https://<DOMAIN>;
    proxy_set_header Host <DOMAIN>;
    proxy_set_header Authorization "Bearer <token>";
    proxy_http_version 1.1;
    proxy_ssl_server_name on;
}
```

```toml
# sentinel config
[server]
url   = "http://<sentry-ip>/watchtower"
token = "proxied"
```

---

## Overall Assessment

The architecture is sound and the monitoring goals are clear. After applying fixes, the full stack works end-to-end: logs reach Loki, OTLP metrics from gnoland reach VictoriaMetrics, and the Grafana dashboards display data. The main blockers were a wrong VictoriaMetrics endpoint in the forwarder and a handful of compatibility issues with the current gnoland test12 version.

---

## Collector Status After Fixes

| Collector | Status | Notes |
|---|---|---|
| RPC | Working | `peers` and `mempool_size` visible in Grafana |
| Logs | Working | Requires `--log-format=json` on gnoland |
| OTLP | Working | Via existing OTel Collector as fan-out |
| Resources | Working | `cpu_percent`, `memory_used_percent`, `disk_used_percent` forwarded |
| Metadata | Working | `telemetry.metrics_enabled` key updated |

---

## Sentinel — Bugs Found and Fixed

### Bug 1 — `journald_linux.go`: `j.Wait()` returns 1 value, not 2

**File:** `internal/sentinel/logs/journald_linux.go:50`

The sentinel fails to compile on Linux:

```
assignment mismatch: 2 variables but j.Wait returns 1 value
```

`sdjournal.Journal.Wait()` in `go-systemd v22.7.0` returns a single `JournalWaitResult` (int), not a `(JournalWaitResult, error)` tuple. Downgrading the dependency did not help — the implementation is out of sync with the current API.

```go
// before
if _, err := j.Wait(time.Second); err != nil {
    return fmt.Errorf("journal wait: %w", err)
}

// after
if r := j.Wait(time.Second); r < 0 {
    return fmt.Errorf("journal wait: error code %d", r)
}
```

**Status: Fixed.**

---

### Bug 2 — `docker.go`: Log collector replays full container history

**File:** `internal/sentinel/logs/docker.go:38`

On startup, the Docker log collector reads and processes the entire container log history because `Tail` is not set in `ContainerLogs`. On a long-running validator this produced 705,787 lines being processed on the first run.

```go
// add Tail option
Tail: "0", // only stream new logs, do not replay history
```

**Status: Fixed.**

---

### Bug 3 — `docker.go`: Non-JSON lines crash the sender

**File:** `internal/sentinel/logs/docker.go:60`

Gnoland emits a few plain-text lines during startup before its JSON logger is initialized. These are stored as `json.RawMessage` without validation and cause a panic during marshaling:

```
json: error calling MarshalJSON for type json.RawMessage: invalid character 'g' looking for beginning of value
```

```go
if !json.Valid(raw) {
    continue // skip non-JSON startup lines silently
}
```

**Status: Fixed.**

---

### Bug 10 — `docker.go`: Log collector does not reconnect after container restart

**File:** `internal/sentinel/logs/docker.go`

When the validator's Docker Compose stack restarts (gnoland update, server reboot), the sentinel loses its connection to the container and stops forwarding data to the watchtower entirely. A manual `systemctl restart sentinel` is required to recover.

Root cause: the Docker log collector holds a long-lived connection to the container via `ContainerLogs`. When the container restarts, that connection is terminated and the collector exits with an error instead of reconnecting automatically.

The RPC collector is not affected — it polls on a ticker and simply logs a warning on each failed attempt, recovering on its own once gnoland is back up.

Fix: wrap the streaming logic in a retry loop. When the container goes away or the connection drops, the collector logs a warning, waits 5 seconds, and attempts to reconnect. Only a context cancellation (sentinel shutdown) exits the loop cleanly.

**Status: Fixed.**

---

### Bug 9 — `metadata/collector.go`: `telemetry.enabled` key no longer exists in gnoland test12

**File:** `internal/sentinel/metadata/collector.go:26`

The `ConfigKeys` list includes `telemetry.enabled`, which was renamed to `telemetry.metrics_enabled` in gnoland test12. This produces a persistent warning on every metadata collection cycle:

```
level=WARN msg="config key error" key=telemetry.enabled err="key \"telemetry.enabled\" not found"
```

Updated `ConfigKeys` to use `telemetry.metrics_enabled`.

**Status: Fixed.**

---

## Watchtower — Bugs Found and Fixed

### Bug 4 — `deploy/Makefile`: `make add-validator` generates empty permissions

**File:** `deploy/Makefile:28`

Running `make add-validator` produces `permissions = []` in `watchtower.toml` instead of the expected list. This causes 403 on every sentinel endpoint immediately after adding a validator.

Root cause: the sed command already wraps the output in double quotes (`"rpc", "metrics", "logs", "otlp"`), and the Makefile wraps it in another pair. The shell receives `""rpc", "metrics"..."` and interprets it as multiple arguments — only the first (empty) string is picked up by `printf`.

```makefile
# buggy — double-quoting
"$(shell echo "$(permissions)" | sed 's/,/", "/g; s/^/"/; s/$$/"/)"

# fixed — store in shell variable to avoid quoting issues
PERMS=$$(echo "$(permissions)" | sed 's/,/", "/g; s/^/"/; s/$$/"/'); \
printf '...' ... "$$PERMS" ...
```

**Status: Fixed.**

---

### Bug 5 — `forwarder.go`: Wrong VictoriaMetrics endpoint and data format (blocking)

**File:** `internal/watchtower/forwarder/forwarder.go:41`

This is the most impactful bug. All RPC, resources, and metadata data fails to reach VictoriaMetrics. The watchtower cascades a 502 back to the sentinel on every request:

```
err="post /api/v1/import/json?...: status 400: unsupported path requested: \"/api/v1/import/json\""
```

Two problems stacked:

1. The endpoint `/api/v1/import/json` does not exist in VictoriaMetrics single-node. The correct endpoint is `/api/v1/import`.
2. The payload format is raw gnoland RPC JSON, not the JSON lines format VictoriaMetrics expects:

```json
// what the sentinel sends (raw gnoland format)
{"collected_at":"...","data":{"net_info":{...},"status":{...}}}

// what VictoriaMetrics expects (JSON lines)
{"metric":{"__name__":"peers","validator":"val-01"},"values":[5],"timestamps":[1234567890000]}
```

The forwarder was rewritten to extract specific numeric fields and emit proper time series:

- `ForwardRPC` — extracts `peers` from `net_info.n_peers`, `mempool_size` from `num_unconfirmed_txs.n_txs`
- `ForwardMetrics` — extracts `cpu_percent`, `memory_used_percent`, `disk_used_percent`
- Both post to `/api/v1/import` in JSON lines format

**Status: Fixed.**

---

### Bug 6 — `forwarder.go`: OTLP handler does not decompress gzip

**File:** `internal/watchtower/forwarder/forwarder.go`

The OTel Collector sends OTLP metrics with gzip compression by default. The watchtower reads the raw body and tries to parse it as protobuf, which fails:

```
err="unmarshal otlp: proto: cannot parse invalid wire-format data"
```

The original design assumed the sentinel's built-in relay would send uncompressed protobuf directly. As soon as an external OTel Collector is used as an intermediary (which is the case here, to preserve dual export to local Prometheus), the default gzip compression breaks parsing.

Immediate workaround: add `compression: none` to the OTel Collector exporter config.

Code fix: auto-detect gzip via magic bytes (`0x1f 0x8b`) and decompress before protobuf parsing.

**Status: Fixed** — gzip auto-detection added in `ForwardOTLP`. The `compression: none` workaround is no longer required.

---

## Configuration Notes

### gnoland must log in JSON format

The sentinel expects structured JSON logs. The gnoland start command must include:

```sh
exec gnoland start --log-format=json ...
```

Without this flag, the log collector sees only plain text and drops everything.

---

### gnoland telemetry key renamed in test12

The config key for enabling OTLP export changed between versions:

```sh
# before (silently ignored — key does not exist in test12)
gnoland config set telemetry.enabled true

# after
gnoland config set telemetry.metrics_enabled true
```

---

### OTel Collector: use `metrics_endpoint`, not `endpoint`

The `otlphttp` exporter has two URL fields with different behaviors:

- `endpoint` — appends `/v1/metrics` automatically → produces the wrong path when the watchtower is behind a subpath like `/watchtower/otlp`
- `metrics_endpoint` — uses the URL as-is → correct

```yaml
exporters:
  otlphttp/watchtower:
    metrics_endpoint: "https://<DOMAIN>/watchtower/otlp"
    headers:
      Authorization: "Bearer <token>"

service:
  pipelines:
    metrics:
      receivers: [otlp]
      processors: [batch]
      exporters: [prometheus, otlphttp/watchtower]
```

Make sure `otlphttp/watchtower` is listed in `service.pipelines.metrics.exporters` — defining it under `exporters:` without referencing it in the pipeline means it is never used.

### OTel Collector behind a sentry: `extra_hosts` required in Docker Compose

When the OTel Collector runs in Docker and forwards to the watchtower through the sentry's nginx reverse proxy, the container cannot resolve the sentry's internal hostname by default. Adding `extra_hosts` in the Docker Compose service injects the mapping directly into the container's `/etc/hosts`:

```yaml
services:
  otel-collector:
    extra_hosts:
      - "sentinel-proxy.internal:172.16.20.2"
```

Without this, the `otlphttp/watchtower` exporter fails to resolve the proxy hostname and the metrics never reach the watchtower.

---

### Loki: retention requires `delete_request_store`

When `retention_enabled: true` is set in Loki's config, the compactor requires a `delete_request_store` field, otherwise Loki fails to start. This is not documented in the example config.

---

### VictoriaMetrics healthcheck: prefer `127.0.0.1` over `localhost`

In some container environments, `localhost` resolves to an IPv6 address while VictoriaMetrics listens on IPv4 only, causing healthchecks to fail. Using `127.0.0.1` explicitly resolves this.

---

## Priority Summary

| Priority | Bug | Component | Status |
|---|---|---|---|
| Critical | Bug 5 — VictoriaMetrics endpoint + data format | Watchtower | **Fixed** |
| High | Bug 4 — `make add-validator` empty permissions | Watchtower | **Fixed** |
| High | Bug 8 — `telemetry.enabled` → `telemetry.metrics_enabled` | Documentation | Documented |
| Medium | Bug 6 — OTLP gzip decompression | Watchtower | **Fixed** |
| Medium | Bug 9 — `telemetry.enabled` in ConfigKeys | Sentinel | **Fixed** |
| Low | Bug 1 — `j.Wait()` compile error | Sentinel | **Fixed** |
| Low | Bug 2 — Docker `Tail:"0"` log history | Sentinel | **Fixed** |
| Low | Bug 3 — `json.Valid()` non-JSON lines | Sentinel | **Fixed** |
| Low | Bug 10 — Docker log collector no auto-reconnect | Sentinel | **Fixed** |
