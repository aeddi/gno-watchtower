# Gno Watchtower

A monitoring system for gnoland validator nodes.

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

- **sentinel** — runs on each validator machine. Collects RPC data, logs, OTLP metrics, resource stats, and node metadata. Ships everything to watchtower over HTTPS, or to a beacon over Noise when the validator sits behind a sentry.
- **beacon** _(optional)_ — runs on a sentry node fronting the validator. Terminates the sentinel's Noise connection, augments each RPC tick with a `sentry_*` view (peer count, build info, p2p.pex), and forwards upstream to watchtower. See [Sentry-fronted setup (with beacon)](#sentry-fronted-setup-with-beacon).
- **watchtower** — runs centrally. Authenticates each sentinel by bearer token, enforces rate limits and IP bans, and forwards to VictoriaMetrics and Loki.
- **Caddy** — TLS termination and reverse proxy. Exposes Grafana and the watchtower API publicly on ports **80**/**443**.
- **VictoriaMetrics** — stores time-series metrics from the RPC and OTLP collectors.
- **Loki** — stores structured logs from the log collector.
- **Grafana** — visualises metrics and logs via provisioned dashboards.

_For the full catalogue of signals collected (per pipeline, cadence, transport, delta semantics), see [docs/data-collected.md](docs/data-collected.md)._

## Prerequisites

**Validator machine:**

- Either Docker ([Option 1](#option-1--docker-recommended)) or a native `sentinel` binary ([Option 2](#option-2--native-binary--systemd))
- Network access to **one** of:
    - the central `watchtower` over HTTPS (port **443**), for direct setups
    - the sentry's `beacon` over TCP (default port **8080**, [configurable](#beacon-config-configtoml)), for [sentry-fronted setups](#sentry-fronted-setup-with-beacon)

**Server:**

- Docker Engine 24+ and Docker Compose v2
- A public domain with DNS pointing to the server (for Caddy TLS)
- Ports **80** and **443** open in the firewall

## Sentinel setup

To ship data to a watchtower, the sentinel needs a bearer token from the central server's admin (see [Adding and removing validators](#adding-and-removing-validators)).

### Option 1 — Docker (recommended)

The image bootstraps its own config and keys on first run, so you can pull and start in a few commands:

```sh
docker pull ghcr.io/aeddi/gno-watchtower/sentinel:latest
sudo mkdir -p /etc/sentinel

docker run -d --name sentinel \
  --restart unless-stopped \
  -v /etc/sentinel:/etc/sentinel \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -p 127.0.0.1:4318:4318 \
  ghcr.io/aeddi/gno-watchtower/sentinel:latest
```

_For image tag conventions, see [Versioning](#versioning)._

On first start the entrypoint writes a default `/etc/sentinel/config.toml` with placeholder values and crash-loops with a clear validation error until you fill the placeholders. Edit `/etc/sentinel/config.toml` and set at least:

- `[server] url` → `https://<DOMAIN>/watchtower`
- `[server] token` → bearer token from [`make add-validator`](#adding-and-removing-validators)
- `[logs] container_name` → your gnoland container name

_For all configurable fields, see [Sentinel config](#sentinel-config-configtoml)._

Then `docker restart sentinel`. Validate end-to-end with:

```sh
docker exec sentinel sentinel doctor /etc/sentinel/config.toml
```

**If your validator is behind a sentry**, see [Sentry-fronted setup (with beacon)](#sentry-fronted-setup-with-beacon) — your Noise keypair is already in `/etc/sentinel/keys`.

### Option 2 — Native binary + systemd

Install via release tarball or `go install`:

```sh
# Either: download a release archive
# https://github.com/aeddi/gno-watchtower/releases
tar -xzf sentinel_<version>_linux_amd64.tar.gz
sudo install -m 0755 sentinel /usr/local/bin/sentinel

# Or: build from source (requires Go 1.25+)
go install github.com/aeddi/gno-watchtower/cmd/sentinel@latest
```

Generate and edit the config:

```sh
sudo mkdir -p /etc/sentinel
sudo sentinel generate-config /etc/sentinel/config.toml
sudo $EDITOR /etc/sentinel/config.toml      # set [server] url, [server] token, [logs] container_name, etc.
sudo sentinel doctor /etc/sentinel/config.toml
```

systemd unit at `/etc/systemd/system/sentinel.service`:

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

_For all configurable fields, see [Sentinel config](#sentinel-config-configtoml)._

Enable and start:

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now sentinel
```

After any later config change, run `sudo systemctl restart sentinel` to apply.

> **journald log source**: tails the gnoland node's logs from systemd-journald (use this when gnoland itself runs as a systemd service writing to journald). The published binaries (release tarballs and `go install` artifacts) are pure-Go (`CGO_ENABLED=0`) and **don't** support `[logs] source = "journald"`. To use it you must build with cgo on a host that has `libsystemd-dev` (or equivalent) installed:
>
> ```sh
> CGO_ENABLED=1 go install github.com/aeddi/gno-watchtower/cmd/sentinel@latest
> ```

**If your validator is behind a sentry**, generate the Noise keypair before `systemctl enable --now sentinel`, then see [Sentry-fronted setup (with beacon)](#sentry-fronted-setup-with-beacon):

```sh
sudo sentinel keygen /etc/sentinel/keys
```

### Sentry-fronted setup (with beacon)

If your validator sits behind a sentry, run a **beacon** on the sentry. The sentinel connects to the beacon over Noise (TCP); the beacon forwards everything upstream and adds a `sentry_*` view (peer count, build info, p2p.pex) to each RPC tick so Grafana can show "direct vs. via-sentry" side-by-side.

```
Validator machine                Sentry machine                  Central server
─────────────────                ──────────────                  ──────────────
┌──────────────┐   Noise (TCP)   ┌────────────┐    HTTPS POST    ┌────────────┐
│   sentinel   │────────────────▶│   beacon   │─────────────────▶│ watchtower │
└──────────────┘                 └─────┬──────┘                  └────────────┘
                                       │ GET /status, /net_info
                                       ▼
                                 sentry gnoland RPC
```

The validator keeps the bearer token; the beacon is a dumb pass-through that only rewrites the `/rpc` body. Tampering with the token is still caught upstream by the watchtower.

**Beacon — Docker (on the sentry):**

```sh
docker pull ghcr.io/aeddi/gno-watchtower/beacon:latest
sudo mkdir -p /etc/beacon

docker run -d --name beacon \
  --restart unless-stopped \
  -v /etc/beacon:/etc/beacon \
  -v /path/to/sentry-config.toml:/sentry-config.toml:ro \
  -p 8080:8080 \
  ghcr.io/aeddi/gno-watchtower/beacon:latest
```

Like the sentinel image, the beacon image bootstraps `/etc/beacon/config.toml` and `/etc/beacon/keys/` on first run. Edit `/etc/beacon/config.toml`:

- `[server] url` → `https://<DOMAIN>/watchtower`
- `[rpc] rpc_url` → sentry's local gnoland RPC (typically `http://localhost:26657`)
- `[metadata] config_path` → `/sentry-config.toml` (the in-container path of the second volume mount above)

Then `docker restart beacon`. The beacon's public key is at `/etc/beacon/keys/pubkey` — copy it for the sentinel side.

**Beacon — Native binary + systemd:** mirror [Option 2](#option-2--native-binary--systemd) above with these substitutions:

- archive: `beacon_<version>_<os>_<arch>.tar.gz` (or `go install github.com/aeddi/gno-watchtower/cmd/beacon@latest`)
- systemd `ExecStart`: `/usr/local/bin/beacon run --log-format=journal /etc/beacon/config.toml`

**Sentinel side:** in `/etc/sentinel/config.toml`, switch the URL scheme to `noise://` and point `[beacon]` at the sentinel's keys directory:

```toml
[server]
url   = "noise://<sentry-host>:8080"
token = "<bearer-token>"

[beacon]
keys_dir   = "/etc/sentinel/keys"
public_key = "<beacon-public-key-hex>"  # optional; pins the beacon's identity
```

The Noise-XX handshake always provides confidentiality. Whether each side authenticates the other is opt-in — see [Beacon authentication modes](#beacon-authentication-modes).

## Server stack setup

Run from `deploy/`:

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

3. Create the initial watchtower config. The deploy stack mounts `watchtower.toml` read-only from the host, so you must create it before `make up`. The defaults below wire watchtower to the in-stack VictoriaMetrics and Loki services and pick sane rate-limit values; override only what you need (see [Watchtower config](#watchtowertoml-server-and-security) for the full schema):

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

5. Grafana is available at `https://<DOMAIN>`. Log in with the admin credentials from `config.env`.

Caddy serves Grafana at `https://<DOMAIN>/` and reverse-proxies `https://<DOMAIN>/watchtower` to the watchtower API — that's why sentinels point their `[server] url` at the `/watchtower` path.

Other Make targets in `deploy/`:

- `make down` — stops the stack
- `make restart svc=<service>` — restarts one service
- `make logs svc=<service>` — follows logs for one service
- `make add-validator` / `make remove-validator` — see [Adding and removing validators](#adding-and-removing-validators)

## Adding and removing validators

Run these commands from the `deploy/` directory.

**Add a validator:**

```sh
make add-validator name=val-01 permissions=rpc,metrics,logs,otlp logs_min_level=info
```

This generates a cryptographically secure bearer token, appends the `[validators.val-01]` block to `watchtower.toml`, restarts watchtower, and prints the token. Paste the token into the sentinel's `config.toml` under `[server] token`.

**Remove a validator:**

```sh
make remove-validator name=val-01
```

This removes the `[validators.val-01]` block from `watchtower.toml` and restarts watchtower. The sentinel will receive 401 responses and stop sending data.

## Beacon authentication modes

The Noise-XX handshake always encrypts the channel. The _server_ is authenticated only when the sentinel pins `[beacon] public_key`; the _client_ is authenticated only when the beacon allowlists the sentinel's public key in `[beacon] authorized_keys`. Both directions are opt-in:

| sentinel `[beacon] public_key` | beacon `[beacon] authorized_keys` | Result                                                                                                          |
| ------------------------------ | --------------------------------- | --------------------------------------------------------------------------------------------------------------- |
| unset                          | empty                             | Encrypted, but either side accepts any peer. Cheapest to deploy; rely on network controls.                      |
| set                            | empty                             | Sentinel pins the beacon; any sentinel may connect to the beacon.                                               |
| unset                          | non-empty                         | Beacon allowlists sentinel public keys; sentinel accepts whatever beacon identity the TCP hostname resolves to. |
| set                            | non-empty                         | Mutual pinning. Recommended for production.                                                                     |

Public keys are at `<keys_dir>/pubkey` (printed on first generation by `keygen`, or by the entrypoint on first container start).

## Configuration reference

### Sentinel config (`config.toml`)

Run `sentinel generate-config <path>` for a fully annotated example. Key fields:

| Section       | Field             | Description                                                                                                |
| ------------- | ----------------- | ---------------------------------------------------------------------------------------------------------- |
| `[server]`    | `url`             | Watchtower base URL (`https://...`) or beacon address (`noise://host:port`)                                |
| `[server]`    | `token`           | Bearer token from [`make add-validator`](#adding-and-removing-validators)                                  |
| `[rpc]`       | `rpc_url`         | Local gnoland RPC (default `http://localhost:26657`)                                                       |
| `[rpc]`       | `poll_interval`   | Poll interval for status/net_info/num_unconfirmed_txs (default `3s`)                                       |
| `[logs]`      | `source`          | `docker` or `journald` (journald requires a cgo build — see [Option 2](#option-2--native-binary--systemd)) |
| `[logs]`      | `container_name`  | Gnoland container name (when `source = "docker"`)                                                          |
| `[logs]`      | `journald_unit`   | systemd unit name (when `source = "journald"`)                                                             |
| `[logs]`      | `min_level`       | Minimum log level to ship                                                                                  |
| `[otlp]`      | `listen_addr`     | Local OTLP listener — point gnoland's exporter here (default `localhost:4318`)                             |
| `[resources]` | `source`          | `host`, `docker`, or `both`                                                                                |
| `[resources]` | `container_name`  | Gnoland container name (when `source = "docker"` or `"both"`)                                              |
| `[metadata]`  | `config_path`     | Path to gnoland's `config.toml` (read for p2p.pex, moniker, etc.)                                          |
| `[metadata]`  | `config_get_cmd`  | Alternative to `config_path` — shell command using `%s` for the key name (set one, not both)               |
| `[self]`      | `report_interval` | Cadence of sentinel self-stats (bytes sent, drops, retries; default `30s`)                                 |
| `[health]`    | `listen_addr`     | Local health endpoint (default `127.0.0.1:8081`; used by the Docker HEALTHCHECK)                           |
| `[beacon]`    | `keys_dir`        | Sentinel Noise keys directory (required when `[server] url` uses `noise://`)                               |
| `[beacon]`    | `public_key`      | Pinned beacon public key (optional, see [authentication modes](#beacon-authentication-modes))              |

### Beacon config (`config.toml`)

Run `beacon generate-config <path>` for the annotated example. Key fields:

| Section      | Field               | Description                                                                                                |
| ------------ | ------------------- | ---------------------------------------------------------------------------------------------------------- |
| `[server]`   | `url`               | Upstream watchtower URL (`https://<DOMAIN>/watchtower`)                                                    |
| `[beacon]`   | `listen_addr`       | Noise listener (default `0.0.0.0:8080`)                                                                    |
| `[beacon]`   | `keys_dir`          | Beacon Noise keys directory                                                                                |
| `[beacon]`   | `authorized_keys`   | Optional allowlist of sentinel public keys (hex), see [authentication modes](#beacon-authentication-modes) |
| `[beacon]`   | `handshake_timeout` | Bound on the Noise handshake phase (default `5s`)                                                          |
| `[rpc]`      | `rpc_url`           | Sentry's local gnoland RPC                                                                                 |
| `[metadata]` | `config_path`       | Sentry's gnoland `config.toml` (for p2p.pex augmentation)                                                  |
| `[metadata]` | `config_get_cmd`    | Alternative to `config_path` — shell command using `%s` for the key name (set one, not both)               |

### `watchtower.toml` (server and security)

| Section              | Field              | Description                                             |
| -------------------- | ------------------ | ------------------------------------------------------- |
| `[server]`           | `listen_addr`      | HTTP listen address (default `0.0.0.0:8080`)            |
| `[security]`         | `rate_limit_rps`   | Per-validator request rate limit                        |
| `[security]`         | `rate_limit_burst` | Token-bucket burst (must be ≥ 4 — sentinel concurrency) |
| `[security]`         | `ban_threshold`    | Failed auth attempts before banning the source IP       |
| `[security]`         | `ban_duration`     | Ban duration (Go duration string, e.g. `15m`)           |
| `[victoria_metrics]` | `url`              | VictoriaMetrics ingest URL                              |
| `[loki]`             | `url`              | Loki ingest URL                                         |

#### Validator block

One `[validators.<name>]` block per sentinel:

| Field            | Description                                                            |
| ---------------- | ---------------------------------------------------------------------- |
| `token`          | Bearer token for this validator (hex string, unique across validators) |
| `permissions`    | Allowed data types: any subset of `["rpc", "metrics", "logs", "otlp"]` |
| `logs_min_level` | Minimum log level forwarded to Loki: `debug`, `info`, `warn`, `error`  |

### `deploy/config.env`

| Variable                 | Description                                        | Default                  |
| ------------------------ | -------------------------------------------------- | ------------------------ |
| `DOMAIN`                 | Public domain for Caddy TLS and Grafana            | `monitoring.example.com` |
| `GRAFANA_ADMIN_USER`     | Grafana admin username                             | `admin`                  |
| `GRAFANA_ADMIN_PASSWORD` | Grafana admin password                             | `changeme`               |
| `METRICS_RETENTION`      | VictoriaMetrics retention in months                | `6`                      |
| `LOGS_RETENTION`         | Loki log retention period (e.g. `2160h` = 90 days) | `2160h`                  |

## Versioning

Releases follow [Semantic Versioning](https://semver.org). Each release publishes:

- **Binaries** on the [Releases page](https://github.com/aeddi/gno-watchtower/releases) for `sentinel`, `beacon`, and `watchtower` — `linux/{amd64,arm64}` and `darwin/{amd64,arm64}`.
- **Docker images** at `ghcr.io/aeddi/gno-watchtower/{sentinel,beacon,watchtower}`. Available tags:
    - `:vX.Y.Z`, `:X.Y.Z`, `:X.Y`, `:latest` — releases
    - `:main`, `:sha-<short>` — main-branch builds (not for production)

Check the version of a running binary with `<bin> version` (or `-v` for commit + build time + Go toolchain).
