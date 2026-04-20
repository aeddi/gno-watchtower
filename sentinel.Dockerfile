# sentinel.Dockerfile — distributable, pure-Go, multi-arch image.
#
# Built and published by .github/workflows/release.yml to
# ghcr.io/aeddi/gno-watchtower/sentinel. Journald log source requires cgo
# + libsystemd and is NOT available in this image; journald users should
# install the native binary via `go install` or a release archive and run
# it under systemd. See README for both paths.
#
# Local build (debug):
#   docker build -f sentinel.Dockerfile --build-arg VERSION=dev -t sentinel:dev .

# ---- Build stage
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build \
        -trimpath \
        -ldflags "-s -w -X github.com/aeddi/gno-watchtower/pkg/version.Version=$VERSION" \
        -o /out/sentinel \
        ./cmd/sentinel

# ---- Runtime stage
FROM alpine:3.21

RUN apk add --no-cache ca-certificates wget

COPY --from=builder /out/sentinel /usr/local/bin/sentinel

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
