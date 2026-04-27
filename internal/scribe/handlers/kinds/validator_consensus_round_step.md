# validator.consensus.round_step — Consensus state-machine step transition

**Source signal type:** log (Loki)
**Gnoland versions:** v0.x onward

## What it represents

Each transition through the BFT consensus state machine at a validator node:
Propose, Prevote, Precommit, and Commit steps. One event is emitted per
step entry, carrying the height, round, and step name.

## How it's detected

The ConsensusRoundStep handler matches gnoland slog lines whose `msg` field
matches the regular expression `enter(\w+)\((\d+)/(\d+)\)`. The captured
groups yield the step name (e.g. `Propose`), block height, and round number.
The `validator` label from the Loki stream identifies the node.

## Linked source code

- gnoland: `tm2/pkg/bft/consensus/state.go` (`enterPropose`, `enterPrevote`,
  `enterPrecommit`, `enterCommit`)

## Payload fields

- `height` (int64) — block height of the consensus round
- `round` (int32) — consensus round number
- `step` (string) — step name (e.g. `Propose`, `Prevote`, `Precommit`, `Commit`)
