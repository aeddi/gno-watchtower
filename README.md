# Gno Watchtower

Gno Watchtower is a lightweight monitoring stack for gnoland validator nodes.
It is composed of two binaries: a **sentinel** that runs on the validator machine
and ships metrics, logs, and OTLP data, and a **watchtower** that receives,
authenticates, and forwards everything to VictoriaMetrics and Loki.

## Table of contents

- [Architecture](#architecture)
- [Prerequisites](#prerequisites)
- [Server stack setup](#server-stack-setup)
- [Sentinel setup](#sentinel-setup)
- [Adding and removing validators](#adding-and-removing-validators)
- [Configuration reference](#configuration-reference)
  - [`deploy/config.env`](#deployconfigenv)
  - [`watchtower.toml` (validator block)](#watchtowertoml-validator-block)
  - [`sentinel` config (`config.toml`)](#sentinel-config-configtoml)
- [Doctor usage](#doctor-usage)
- [FAQ](#faq)
  - [Deploying behind a sentry node (private network)](#deploying-behind-a-sentry-node-private-network)
  - [Using an existing OTel Collector as a fan-out](#using-an-existing-otel-collector-as-a-fan-out)
  - [OTel Collector in Docker, sentry unreachable](#my-otel-collector-runs-in-docker-and-cannot-reach-the-sentry-proxy)

---

## Architecture

```
Validator machine(s)                    Central server
────────────────────                    ─────────────────────────────────────────────────
┌──────────────────┐                    ┌────────┐  ┌─────────────┐
│    sentinel      │                    │        │  │ watchtower  │   ┌─────────────────┐
│ ──────────────── │     HTTPS POST     │        │  │ ─────────── │──▶│ Loki +          │
│ RPC collector    │───────────────────▶│ Caddy  │─▶│ auth        │──▶│ VictoriaMetrics │
│ Log collector    │                    │ (TLS)  │  │ rate limit  │   └─────────────────┘
│ OTLP relay       │                    │        │  │ IP ban      │       ┌─────────┐  │
│ Resource monitor │                    └────────┘  └─────────────┘       │ Grafana │◀─┘
│ Metadata         │                                                      └─────────┘
└──────────────────┘
```

- **sentinel** — runs on each validator machine. Collects RPC data, logs, OTLP metrics, resource stats, and node metadata. Ships everything to watchtower over HTTPS.
- **watchtower** — runs centrally. Authenticates each sentinel by bearer token, enforces rate limits and IP bans, and forwards to VictoriaMetrics and Loki.
- **Caddy** — TLS termination and reverse proxy. Exposes Grafana and the watchtower API publicly on ports 80/443.
- **VictoriaMetrics** — stores time-series metrics from the RPC and OTLP collectors.
- **Loki** — stores structured logs from the log collector.
- **Grafana** — visualises metrics and logs via provisioned dashboards.

---

## Prerequisites

**Server:**

- Docker Engine 24+ and Docker Compose v2
- A public domain with DNS pointing to the server (for Caddy TLS)
- Ports 80 and 443 open in the firewall

**Validator machine:**

- Linux (not tested on macOS)
- gnoland Docker image or standalone binary
- `sentinel` binary
- Network access to the central server on port 443

---

## Server stack setup

1. Clone the repository:

    ```sh
    git clone https://github.com/gnolang/gno-watchtower.git
    cd gno-watchtower/deploy
    ```

2. Copy and edit the environment file:

    ```sh
    cp config.env.example config.env
    $EDITOR config.env   # set DOMAIN, GRAFANA_ADMIN_PASSWORD
    ```

3. Create the initial (empty) watchtower config:

    ```sh
    cat > watchtower.toml <<'EOF'
    [server]
    listen_addr = "0.0.0.0:8080"

    [security]
    rate_limit_rps   = 10
    # rate_limit_burst must be >= number of concurrent sentinel data types (rpc + metrics + logs + otlp = 4)
    rate_limit_burst = 10
    ban_threshold    = 5
    ban_duration     = "15m"

    [victoria_metrics]
    url = "http://victoria-metrics:8428"

    [loki]
    url = "http://loki:3100"
    EOF
    ```

4. Start the stack:

    ```sh
    make up
    ```

5. Grafana is available at `https://<DOMAIN>`. Log in with the admin credentials set in `config.env`.

---

## Sentinel setup

> [!WARNING]
> The validator must generate logs in JSON format. Add the flag when starting gnoland:
> ```sh
> gnoland start --log-format=json ...
> ```

1. Build the binary:

    ```sh
    git clone https://github.com/gnolang/gno-watchtower.git
    cd gno-watchtower
    go build -o sentinel ./cmd/sentinel
    ```

2. Copy the `sentinel` binary to the validator machine.

3. Generate an example config:

    ```sh
    sentinel generate-config > /etc/sentinel/config.toml
    $EDITOR /etc/sentinel/config.toml
    ```

4. Set `[server] url` to `https://<DOMAIN>/watchtower` and `token` to the value printed by `make add-validator` (see [Adding and removing validators](#adding-and-removing-validators)).

5. Run sentinel:

    ```sh
    sentinel run /etc/sentinel/config.toml
    ```

    For production, run as a systemd service:

    ```ini
    [Unit]
    Description=Gnoland Sentinel
    After=network.target

    [Service]
    ExecStart=/usr/local/bin/sentinel run --log-format=journal /etc/sentinel/config.toml
    Restart=on-failure

    [Install]
    WantedBy=multi-user.target
    ```

---

## Adding and removing validators

Run these commands from the `deploy/` directory.

**Add a validator:**

```sh
make add-validator name=val-01 permissions=rpc,metrics,logs,otlp logs_min_level=info
```

This generates a cryptographically secure bearer token, appends the `[validators.val-01]` block to `watchtower.toml`, restarts watchtower, and prints the token. Paste the token into the sentinel's `config.toml` `[server] token` field.

**Remove a validator:**

```sh
make remove-validator name=val-01
```

This removes the `[validators.val-01]` block from `watchtower.toml` and restarts watchtower. The sentinel will receive 401 responses and stop sending data.

---

## Configuration reference

### `deploy/config.env`

| Variable                 | Description                                        | Default                  |
| ------------------------ | -------------------------------------------------- | ------------------------ |
| `DOMAIN`                 | Public domain for Caddy TLS and Grafana            | `monitoring.example.com` |
| `GRAFANA_ADMIN_USER`     | Grafana admin username                             | `admin`                  |
| `GRAFANA_ADMIN_PASSWORD` | Grafana admin password                             | `changeme`               |
| `METRICS_RETENTION`      | VictoriaMetrics retention in months                | `6`                      |
| `LOGS_RETENTION`         | Loki log retention period (e.g. `2160h` = 90 days) | `2160h`                  |

### `watchtower.toml` (validator block)

| Field            | Description                                                            |
| ---------------- | ---------------------------------------------------------------------- |
| `token`          | Bearer token for this validator (hex string)                           |
| `permissions`    | Allowed data types: any subset of `["rpc", "metrics", "logs", "otlp"]` |
| `logs_min_level` | Minimum log level forwarded to Loki: `debug`, `info`, `warn`, `error`  |

### `sentinel` config (`config.toml`)

Run `sentinel generate-config` for a fully annotated example. Key fields:

| Section       | Field         | Description                              |
| ------------- | ------------- | ---------------------------------------- |
| `[server]`    | `url`         | Watchtower base URL                      |
| `[server]`    | `token`       | Bearer token from `make add-validator`   |
| `[logs]`      | `source`      | `docker` or `journald`                   |
| `[logs]`      | `min_level`   | Minimum log level to ship                |
| `[otlp]`      | `listen_addr` | Local OTLP listener (point gnoland here) |
| `[resources]` | `source`      | `host`, `docker`, or `both`              |

---

## Doctor usage

`sentinel doctor` checks your sentinel configuration and actual runtime environment, then prints a colour-coded report:

| Symbol | Meaning                       |
| ------ | ----------------------------- |
| ✅     | Working correctly             |
| ❌     | Enabled but failing           |
| 🟠     | Disabled in config            |
| ⬜     | Not permitted by server token |

Run it before deploying or after any configuration change:

```sh
sentinel doctor /etc/sentinel/config.toml
```

Checks performed: metadata binary/genesis/config access, metadata path/cmd conflicts, log visibility and JSON format, OTLP connectivity, resource access, remote reachability, token validity, and token permissions alignment.

---

## FAQ

### Deploying behind a sentry node (private network)

This is a common setup: the validator runs in an isolated network, and a **sentry node** sits between the validator and the public internet for P2P relay. The sentinel on the validator cannot reach the watchtower directly.

**Solution:** configure nginx on the sentry as a reverse proxy. The proxy forwards the sentinel's requests to the watchtower and injects the real Bearer token — the validator machine never needs to hold it.

**Sentry — nginx configuration:**

```nginx
location /watchtower/ {
    allow <validator-private-ip>;
    deny all;

    proxy_pass          https://<DOMAIN>;
    proxy_set_header    Host <DOMAIN>;
    proxy_set_header    Authorization "Bearer <token>";
    proxy_http_version  1.1;
    proxy_ssl_server_name on;
}
```

**Validator — sentinel `config.toml`:**

```toml
[server]
url   = "http://<sentry-private-ip>/watchtower"
token = "proxied"   # the real token is injected by nginx, any non-empty value works
```

The sentinel sends its requests to the sentry over the private network (plain HTTP), the sentry upgrades to HTTPS and authenticates against the watchtower.

---

### Using an existing OTel Collector as a fan-out

No. If an OTel Collector is already present, you can use it as a fan-out: it receives gnoland's OTLP metrics and forwards them both to your local Prometheus and to the watchtower simultaneously. The sentinel's built-in `[otlp]` relay can be left disabled.

**OTel Collector exporter config:**

```yaml
exporters:
  otlphttp/watchtower:
    metrics_endpoint: "https://<DOMAIN>/watchtower/otlp"
    compression: none
    headers:
      Authorization: "Bearer <token>"

service:
  pipelines:
    metrics:
      receivers: [otlp]
      processors: [batch]
      exporters: [prometheus, otlphttp/watchtower]
```

> [!IMPORTANT]
> Set `compression: none` on the exporter. By default the OTel Collector compresses outgoing OTLP payloads with gzip. The watchtower auto-detects and decompresses gzip, but setting `compression: none` avoids any compatibility issue with intermediaries (e.g. the sentry nginx proxy) that may not forward compressed bodies correctly.

> [!IMPORTANT]
> Use `metrics_endpoint`, not `endpoint`. The `endpoint` field appends `/v1/metrics` automatically, which produces the wrong path when the watchtower sits behind a subpath like `/watchtower/otlp`.

> [!IMPORTANT]
> Make sure `otlphttp/watchtower` is listed under `service.pipelines.metrics.exporters`. Defining an exporter block without referencing it in the pipeline means it is never used.

---

### My OTel Collector runs in Docker and cannot reach the sentry proxy

When the OTel Collector runs as a Docker container and forwards metrics through the sentry's nginx, the container's DNS may not resolve the sentry's internal hostname. Inject the mapping via `extra_hosts` in your `docker-compose.yml`:

```yaml
services:
  otel-collector:
    extra_hosts:
      - "<sentry-hostname>:<sentry-private-ip>"
```

This writes the entry directly into the container's `/etc/hosts` at startup.

---
