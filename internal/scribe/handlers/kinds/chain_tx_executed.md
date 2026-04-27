# chain.tx_executed — Transaction executed

**Source signal type:** log (Loki)
**Gnoland versions:** v0.x onward

## What it represents

A transaction was processed by the chain. Currently only the rejection
(failure) path is captured. gnoland does not emit a structured per-transaction
success log line; the success path is not implemented and awaits Phase-14
replay test validation.

## How it's detected

The TxExecuted handler matches gnoland slog lines whose `msg` field contains
`"Rejected bad transaction"`. The `height` field records the block at which
the rejection occurred and the `err` field provides the rejection reason.
The subject is set to `types.SubjectChain`. The `type` field is fixed to
`"unknown"` pending richer log data.

## Linked source code

- gnoland: `tm2/pkg/bft/mempool/reactor.go` (transaction rejection)

## Payload fields

- `height` (int64) — block height at which the transaction was rejected
- `type` (string) — transaction type (`"unknown"` until phase-14 data is available)
- `success` (bool) — always `false` for the currently handled rejection path
- `error` (string) — rejection error message
