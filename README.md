# Gno Watchtower

A two-binary (sentinel/watchtower) monitoring system for gnoland validator nodes.

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

## Server stack setup

1. Clone the repository:

    ```sh
    git clone https://github.com/aeddi/gno-watchtower.git
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

## Sentinel setup

1. Copy the `sentinel` binary to the validator machine.

2. Generate an example config:

    ```sh
    sentinel generate-config > /etc/sentinel/config.toml
    $EDITOR /etc/sentinel/config.toml
    ```

3. Set `[server] url` to `https://<DOMAIN>/watchtower` and `token` to the value printed by `make add-validator` (see below).

4. Run sentinel:

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

## Doctor usage

`sentinel doctor` checks your sentinel configuration and actual runtime environment, then prints a colour-coded report:

| Symbol | Meaning                       |
| ------ | ----------------------------- |
| GREEN  | Working correctly             |
| RED    | Enabled but failing           |
| ORANGE | Disabled in config            |
| GREY   | Not permitted by server token |

Run it before deploying or after any configuration change:

```sh
sentinel doctor /etc/sentinel/config.toml
```

Checks performed: metadata binary/genesis/config access, metadata path/cmd conflicts, log visibility and JSON format, OTLP connectivity, resource access, remote reachability, token validity, and token permissions alignment.
