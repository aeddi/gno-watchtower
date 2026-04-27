# validator.proposed — Validator selected as proposer

**Source signal type:** log (Loki)
**Gnoland versions:** v0.x onward

## What it represents

A validator was chosen as the proposer for a consensus round. This event is
emitted once per proposal opportunity, recording the block height and round
number at which the validator had the right to propose.

## How it's detected

The Proposed handler matches gnoland slog lines whose `msg` field contains
`"Our turn to propose"`. The `height` and `round` fields are extracted from
the JSON-structured log line. The `validator` label from the Loki stream
identifies which node produced the log.

## Linked source code

- gnoland: `tm2/pkg/bft/consensus/state.go` (`enterPropose` step)

## Payload fields

- `height` (int64) — block height of the proposal round
- `round` (int32) — consensus round number
