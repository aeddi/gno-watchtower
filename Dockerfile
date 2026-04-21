# syntax=docker/dockerfile:1.7
#
# Multi-target Dockerfile for every gno-watchtower binary.
#
# Targets:
#   watchtower — central server (deploy/docker-compose.yml builds this)
#   sentinel   — validator-side collector (published to ghcr.io/.../sentinel)
#   beacon     — sentry-side Noise relay (published to ghcr.io/.../beacon)
#
# All three share a single `base` stage that does `go mod download` + copies
# the source tree once. Per-binary `builder-*` stages then compile only their
# binary; `docker build --target <name>` picks the binary and its runtime.
#
# Local builds:
#   docker build --target sentinel   --build-arg VERSION=dev -t sentinel:dev .
#   docker build --target beacon     --build-arg VERSION=dev -t beacon:dev .
#   docker build --target watchtower --build-arg VERSION=dev -t watchtower:dev .

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
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build \
        -trimpath \
        -ldflags "-s -w -X github.com/aeddi/gno-watchtower/pkg/version.Version=$VERSION" \
        -o /out/sentinel \
        ./cmd/sentinel

FROM base AS builder-beacon
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build \
        -trimpath \
        -ldflags "-s -w -X github.com/aeddi/gno-watchtower/pkg/version.Version=$VERSION" \
        -o /out/beacon \
        ./cmd/beacon

FROM base AS builder-watchtower
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build \
        -trimpath \
        -ldflags "-s -w -X github.com/aeddi/gno-watchtower/pkg/version.Version=$VERSION" \
        -o /out/watchtower \
        ./cmd/watchtower

# ---- Runtime: sentinel
#
# Journald log source requires cgo + libsystemd and is NOT available in this
# image; journald users should install the native binary via `go install` or
# a release archive and run it under systemd. See README for both paths.
FROM alpine:3.21 AS sentinel
RUN apk add --no-cache ca-certificates wget
COPY --from=builder-sentinel /out/sentinel /usr/local/bin/sentinel
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
ENTRYPOINT ["/usr/local/bin/sentinel"]
CMD ["run", "/etc/sentinel/config.toml"]

# ---- Runtime: beacon
FROM alpine:3.21 AS beacon
RUN apk add --no-cache ca-certificates
COPY --from=builder-beacon /out/beacon /usr/local/bin/beacon
ARG VERSION=dev
LABEL org.opencontainers.image.title="gno-watchtower beacon" \
      org.opencontainers.image.description="Sentry-side Noise relay for gno-watchtower monitoring — forwards sentinel traffic to the central watchtower and augments /rpc payloads with the sentry's own view of the network." \
      org.opencontainers.image.source="https://github.com/aeddi/gno-watchtower" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="$VERSION"
# Noise listener accepting sentinel connections.
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/beacon"]
CMD ["run", "/etc/beacon/config.toml"]

# ---- Runtime: watchtower
#
# Built locally by deploy/docker-compose.yml; not published to GHCR. The
# watchtower is the central ingest/auth/forward service and runs exclusively
# inside the self-hosted server stack.
FROM alpine:3.21 AS watchtower
RUN apk add --no-cache ca-certificates wget
COPY --from=builder-watchtower /out/watchtower /usr/local/bin/watchtower
ARG VERSION=dev
LABEL org.opencontainers.image.title="gno-watchtower" \
      org.opencontainers.image.description="Central ingest/auth/forward service for gno-watchtower." \
      org.opencontainers.image.source="https://github.com/aeddi/gno-watchtower" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="$VERSION"
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/health || exit 1
ENTRYPOINT ["/usr/local/bin/watchtower"]
CMD ["run", "/etc/watchtower/config.toml"]
