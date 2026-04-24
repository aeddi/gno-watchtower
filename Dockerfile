# syntax=docker/dockerfile:1.7
#
# Multi-target Dockerfile for every gno-watchtower binary.
#
# Targets (all three published to ghcr.io/aeddi/gno-watchtower/<name>):
#   sentinel   — validator-side collector
#   beacon     — sentry-side Noise relay
#   watchtower — central ingest/auth/forward service (also built locally by
#                deploy/docker-compose.yml to run the self-hosted server stack)
#
# All three share a single `base` stage that does `go mod download` + copies
# the source tree once. Per-binary `builder-*` stages then compile only their
# binary; `docker build --target <name>` picks the binary and its runtime.
#
# Local builds:
#   docker build --target sentinel \
#     --build-arg VERSION=dev \
#     --build-arg COMMIT=$(git rev-parse HEAD) \
#     --build-arg BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
#     -t sentinel:dev .
#   (same pattern for --target beacon / watchtower)

# ---- Shared base (source + go mod cache)
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS base
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# ---- Per-binary builder stages
FROM base AS builder-sentinel
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=""
ARG BUILD_TIME=""
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
  go build \
  -trimpath \
  -ldflags "-s -w \
    -X github.com/aeddi/gno-watchtower/pkg/version.Version=$VERSION \
    -X github.com/aeddi/gno-watchtower/pkg/version.Commit=$COMMIT \
    -X github.com/aeddi/gno-watchtower/pkg/version.Built=$BUILD_TIME" \
  -o /out/sentinel \
  ./cmd/sentinel

FROM base AS builder-beacon
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=""
ARG BUILD_TIME=""
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
  go build \
  -trimpath \
  -ldflags "-s -w \
    -X github.com/aeddi/gno-watchtower/pkg/version.Version=$VERSION \
    -X github.com/aeddi/gno-watchtower/pkg/version.Commit=$COMMIT \
    -X github.com/aeddi/gno-watchtower/pkg/version.Built=$BUILD_TIME" \
  -o /out/beacon \
  ./cmd/beacon

FROM base AS builder-watchtower
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=""
ARG BUILD_TIME=""
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
  go build \
  -trimpath \
  -ldflags "-s -w \
    -X github.com/aeddi/gno-watchtower/pkg/version.Version=$VERSION \
    -X github.com/aeddi/gno-watchtower/pkg/version.Commit=$COMMIT \
    -X github.com/aeddi/gno-watchtower/pkg/version.Built=$BUILD_TIME" \
  -o /out/watchtower \
  ./cmd/watchtower

# ---- Runtime: sentinel
#
# Journald log source requires cgo + libsystemd and is NOT available in this
# image; journald users should install the native binary via `go install` or
# a release archive and run it under systemd. See README for both paths.
#
# Runs as root by design: when `[logs] source = "docker"` is configured the
# sentinel reads /var/run/docker.sock, which is root-owned on most hosts.
# Non-root operation would require `--group-add $(stat -c %g docker.sock)`
# at `docker run` time, which is an operator-visible footgun. The container
# has no other attack surface beyond reading that socket and making HTTPS
# calls to the configured watchtower.
FROM alpine:3.21 AS sentinel
RUN apk add --no-cache ca-certificates wget
COPY --from=builder-sentinel /out/sentinel /usr/local/bin/sentinel
COPY --chmod=0755 docker-entrypoint.sh /usr/local/bin/entrypoint.sh
ARG VERSION=dev
LABEL org.opencontainers.image.title="gno-watchtower sentinel" \
  org.opencontainers.image.description="Sentinel sidecar for gno-watchtower monitoring — collects gnoland RPC / logs / OTLP / resources and ships them to the watchtower." \
  org.opencontainers.image.source="https://github.com/aeddi/gno-watchtower" \
  org.opencontainers.image.licenses="MIT" \
  org.opencontainers.image.version="$VERSION"
# OTLP relay (gnoland → sentinel) and the optional health endpoint.
EXPOSE 4318 8081
# HEALTHCHECK assumes [health] enabled = true in config.toml (the default).
# Override in docker-compose.yml if health is disabled.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8081/health || exit 1
ENTRYPOINT ["/usr/local/bin/entrypoint.sh", "sentinel"]
CMD ["run", "/etc/sentinel/config.toml"]

# ---- Runtime: beacon
FROM alpine:3.21 AS beacon
RUN apk add --no-cache ca-certificates && \
  addgroup -S -g 10001 app && \
  adduser -S -u 10001 -G app app && \
  mkdir -p /etc/beacon && chown -R app:app /etc/beacon
COPY --from=builder-beacon /out/beacon /usr/local/bin/beacon
COPY --chmod=0755 docker-entrypoint.sh /usr/local/bin/entrypoint.sh
ARG VERSION=dev
LABEL org.opencontainers.image.title="gno-watchtower beacon" \
  org.opencontainers.image.description="Sentry-side Noise relay for gno-watchtower monitoring — forwards sentinel traffic to the central watchtower and augments /rpc payloads with the sentry's own view of the network." \
  org.opencontainers.image.source="https://github.com/aeddi/gno-watchtower" \
  org.opencontainers.image.licenses="MIT" \
  org.opencontainers.image.version="$VERSION"
# Noise listener accepting sentinel connections.
EXPOSE 8080
USER app
ENTRYPOINT ["/usr/local/bin/entrypoint.sh", "beacon"]
CMD ["run", "/etc/beacon/config.toml"]

# ---- Runtime: watchtower
FROM alpine:3.21 AS watchtower
RUN apk add --no-cache ca-certificates wget && \
  addgroup -S -g 10001 app && \
  adduser -S -u 10001 -G app app && \
  mkdir -p /etc/watchtower && chown -R app:app /etc/watchtower
COPY --from=builder-watchtower /out/watchtower /usr/local/bin/watchtower
COPY --chmod=0755 docker-entrypoint.sh /usr/local/bin/entrypoint.sh
ARG VERSION=dev
LABEL org.opencontainers.image.title="gno-watchtower" \
  org.opencontainers.image.description="Central ingest/auth/forward service for gno-watchtower." \
  org.opencontainers.image.source="https://github.com/aeddi/gno-watchtower" \
  org.opencontainers.image.licenses="MIT" \
  org.opencontainers.image.version="$VERSION"
EXPOSE 8080
USER app
HEALTHCHECK --interval=10s --timeout=5s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/health || exit 1
ENTRYPOINT ["/usr/local/bin/entrypoint.sh", "watchtower"]
CMD ["run", "/etc/watchtower/config.toml"]
