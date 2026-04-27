# chain.block_committed — Block executed and committed

**Source signal type:** log (Loki)
**Gnoland versions:** v0.x onward

## What it represents

A block was executed by the state machine and committed to the chain. One
event is emitted per block, carrying the height and total transaction count
(valid + invalid). The proposer field is not available in this log line and
is left empty pending Phase-14 replay test validation.

## How it's detected

The BlockCommitted handler matches gnoland slog lines whose `msg` field
contains `"Executed block"`. The `height`, `validTxs`, and `invalidTxs`
fields are extracted from the JSON log line; the total `txs` is their sum.
The subject is set to `types.SubjectChain` since this is a chain-level event.

## Linked source code

- gnoland: `tm2/pkg/bft/state/execution.go` (`ApplyBlock`)

## Payload fields

- `height` (int64) — committed block height
- `txs` (int32) — total number of transactions (valid + invalid)
