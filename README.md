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

Choose one of the three install paths below. All three read the same `config.toml`, so the "Generate and edit config" step is identical.

> **Journald log source**: only available in native binary builds (systemd + libsystemd). The published Docker image is pure-Go and does **not** link libsystemd — configs with `[logs] source = "journald"` will fail at startup inside the container. Pick Option 2 (native + systemd) if you need journald.

### Generate and edit config (all three options)

```sh
# from whichever option you've installed the binary with
sentinel generate-config /etc/sentinel/config.toml
$EDITOR /etc/sentinel/config.toml
```

Set:
- `[server] url` → `https://<DOMAIN>/watchtower`
- `[server] token` → the value printed by `make add-validator` (see [Adding and removing validators](#adding-and-removing-validators))
- `[logs] source` → `docker` (default) or `journald` (native-binary only)

Validate with `sentinel doctor /etc/sentinel/config.toml` before starting.

### Option 1 — Docker (recommended for most setups)

```sh
# Pull once
docker pull ghcr.io/aeddi/gno-watchtower/sentinel:latest

# Run (restarts on failure, logs persist)
docker run -d --name sentinel \
  --restart unless-stopped \
  -v /etc/sentinel/config.toml:/etc/sentinel/config.toml:ro \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -p 127.0.0.1:4318:4318 \
  ghcr.io/aeddi/gno-watchtower/sentinel:latest
```

- The `docker.sock` mount is required only when `[logs] source = "docker"`.
- Pin a specific version (`:v0.1.0`) in production; `:latest` tracks the newest release; `:main` is the bleeding-edge main-branch build (not for production).
- Bleeding-edge and per-commit images: `:sha-<short>`.

### Option 2 — Native binary + systemd (required for journald)

Download a prebuilt binary from the [GitHub Releases](https://github.com/aeddi/gno-watchtower/releases) page. Pick the archive matching your OS + arch (e.g. `sentinel_v0.1.0_linux_amd64.tar.gz`). Extract and install:

```sh
tar -xzf sentinel_v0.1.0_linux_amd64.tar.gz
sudo install -m 0755 sentinel /usr/local/bin/sentinel
```

systemd service:

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

Journald log source requires libsystemd to be present at run time (usually already installed on any systemd host).

### Option 3 — `go install` (development)

```sh
go install github.com/aeddi/gno-watchtower/cmd/sentinel@latest
sentinel version   # confirm the install
```

Go installs into `$GOBIN` (defaults to `$GOPATH/bin`). Pin a version with `@v0.1.0` for reproducibility. Journald support is **not** built in this path unless you set `CGO_ENABLED=1` and have `libsystemd-dev` (or equivalent) installed.

## Beacon setup (optional, for sentry-fronted validators)

If your validator sits behind a sentry node, you can run a **beacon** on the sentry. The sentinel on the validator connects to the beacon over Noise, and the beacon forwards everything upstream to the central watchtower, adding a `sentry_*` view (peer count, build info, p2p.pex) to each RPC tick so Grafana can show "direct vs. via-sentry" side-by-side.

```
Validator machine                Sentry machine                Central server
──────────────────               ──────────────               ───────────────
┌──────────────┐  Noise (TCP)   ┌────────────┐  HTTPS POST   ┌────────────┐
│   sentinel   │───────────────▶│   beacon   │──────────────▶│ watchtower │
└──────────────┘                └─────┬──────┘               └────────────┘
                                      │ GET /status,/net_info
                                      ▼
                                 sentry gnoland RPC
```

The validator keeps the bearer token; the beacon is a dumb pass-through that only rewrites the `/rpc` body. Any tampering with the token is still caught by the watchtower.

### 1. Generate keypairs

Both sides get one static Curve25519 keypair. The public keys are exchanged out-of-band (once, at install time).

```sh
# On the sentry machine (where the beacon runs)
beacon keygen /etc/beacon/keys
# Prints the beacon's public key (hex) — copy it.

# On the validator machine (where the sentinel runs)
sentinel keygen /etc/sentinel/keys
# Prints the sentinel's public key (hex) — copy it.
```

### 2. Configure the sentinel

In the sentinel's `config.toml`:

```toml
[server]
url   = "noise://<sentry-host>:8080"
token = "<bearer-token-from-make-add-validator>"

[beacon]
keys_dir   = "/etc/sentinel/keys"
public_key = "<beacon-public-key-hex>"  # optional; pins the beacon's identity
```

### 3. Configure the beacon

Generate a template and edit:

```sh
beacon generate-config /etc/beacon/config.toml
$EDITOR /etc/beacon/config.toml
```

Set:
- `[server] url` → `https://<DOMAIN>/watchtower`
- `[beacon] listen_addr` → `0.0.0.0:8080`
- `[beacon] keys_dir` → `/etc/beacon/keys`
- `[rpc] rpc_url` → the sentry's local gnoland RPC (typically `http://localhost:26657`)
- `[metadata] config_path` → the sentry's `config.toml` (for p2p.pex augmentation)

### Authentication modes

The Noise-XX handshake always provides confidentiality and authentication of the *server* when the sentinel pins `[beacon] public_key`. Peer-of-peer authentication is opt-in on each side:

| sentinel `[beacon] public_key` | beacon `[beacon] authorized_keys` | Result |
| --- | --- | --- |
| unset | empty | Encrypted, but either side accepts any peer. Cheapest to deploy; rely on network controls. |
| set | empty | Sentinel pins the beacon; any sentinel may connect to the beacon. |
| unset | non-empty | Beacon allowlists sentinel public keys; sentinel accepts whatever beacon identity the TCP hostname resolves to. |
| set | non-empty | Mutual pinning. Recommended for production. |

### 4. Run the beacon

**Docker:**

```sh
docker pull ghcr.io/aeddi/gno-watchtower/beacon:latest

docker run -d --name beacon \
  --restart unless-stopped \
  -v /etc/beacon/config.toml:/etc/beacon/config.toml:ro \
  -v /etc/beacon/keys:/etc/beacon/keys:ro \
  -v /path/to/sentry/config.toml:/sentry-config.toml:ro \
  -p 8080:8080 \
  ghcr.io/aeddi/gno-watchtower/beacon:latest
```

The third mount exposes the sentry's gnoland `config.toml` to the beacon so it can read `p2p.pex` and related keys — point `[metadata] config_path` at `/sentry-config.toml` in that case.

**Native + systemd:**

```sh
tar -xzf beacon_v0.1.0_linux_amd64.tar.gz
sudo install -m 0755 beacon /usr/local/bin/beacon
```

```ini
[Unit]
Description=Gnoland Beacon
After=network.target

[Service]
ExecStart=/usr/local/bin/beacon run --log-format=journal /etc/beacon/config.toml
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

Checks performed: metadata config access, metadata path/cmd conflicts, log visibility and JSON format, OTLP connectivity, resource access, remote reachability, token validity, and token permissions alignment.

## Versioning

Releases follow [Semantic Versioning](https://semver.org) and are driven by [Conventional Commits](https://www.conventionalcommits.org). [release-please](https://github.com/googleapis/release-please) opens a Release PR whenever unreleased `feat:` / `fix:` commits accumulate on `main`; merging the Release PR cuts the tag and triggers CI to build binaries and push the Docker image.

Artifacts per release:

- **Binaries** on the [Releases page](https://github.com/aeddi/gno-watchtower/releases): `sentinel_<version>_<os>_<arch>.tar.gz` and `watchtower_<version>_<os>_<arch>.tar.gz` for `linux/{amd64,arm64}` and `darwin/{amd64,arm64}`
- **Docker image** at `ghcr.io/aeddi/gno-watchtower/sentinel` tagged `:<version>`, `:<major>.<minor>`, `:latest` (plus `:main` and `:sha-<short>` for non-release builds)

Check the version of a running binary with `sentinel version` (or `-v` for commit + build time + Go toolchain).
