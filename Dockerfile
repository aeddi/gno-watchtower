FROM golang:1.25-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o watchtower ./cmd/watchtower

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /build/watchtower /usr/local/bin/watchtower
ENTRYPOINT ["watchtower"]
