# validator.height_advanced — Validator block height advance

**Source signal type:** metric (VictoriaMetrics)
**Gnoland versions:** v0.x onward

## What it represents

The latest committed block height as reported by a validator. An event is
emitted each time the per-validator height increases; a `samples_validator`
row is upserted on every poll regardless of whether the height changed.

## How it's detected

The Height handler matches the OTLP metric named
`sentinel_rpc_latest_block_height` emitted by each validator's sentinel
collector. On each VictoriaMetrics observation the handler upserts a
`samples_validator` row with the height column set. When the height value
exceeds the previously seen value for that validator, a
`validator.height_advanced` event is also inserted.

## Linked source code

- gnoland: `tm2/pkg/bft/consensus/state.go` (block commit broadcast)
- sentinel: `internal/sentinel/collector/rpc.go` (RPC status scrape)

## Payload fields

- `from` (int64) — previous block height
- `to` (int64) — new block height
