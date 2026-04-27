# validator.peers — Validator peer counts

**Source signal type:** metric (VictoriaMetrics)
**Gnoland versions:** v0.x onward

## What it represents

The number of inbound and outbound peer connections as observed per validator
node. This handler only upserts `samples_validator` rows; it does not emit
events.

## How it's detected

The Peers handler matches two OTLP metrics emitted by gnoland:
`inbound_peers_gauge` and `outbound_peers_gauge`. Both carry a `{validator}`
label. The direction is encoded in the metric name rather than a label.
Each observation upserts the relevant peer-count column in `samples_validator`
with a microsecond offset to avoid primary-key collisions with other handlers
writing at the same metric timestamp.

## Linked source code

- gnoland: `tm2/pkg/p2p/switch.go` (peer count gauges)
- sentinel: `internal/sentinel/collector/otlp.go` (OTLP forwarding)

## Payload fields

No event payload — this handler only produces sample upserts:

- `peer_count_in` (int16) — number of inbound peers
- `peer_count_out` (int16) — number of outbound peers
