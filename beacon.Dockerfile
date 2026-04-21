# beacon.Dockerfile — distributable, pure-Go, multi-arch image.
#
# Built and published by .github/workflows/release.yml to
# ghcr.io/aeddi/gno-watchtower/beacon. The beacon is a stateless relay
# between a validator's sentinel and the watchtower — no cgo, no journald,
# no gnoland dependency.
#
# Local build (debug):
#   docker build -f beacon.Dockerfile --build-arg VERSION=dev -t beacon:dev .

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
        -o /out/beacon \
        ./cmd/beacon

# ---- Runtime stage
FROM alpine:3.21

RUN apk add --no-cache ca-certificates

COPY --from=builder /out/beacon /usr/local/bin/beacon

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
