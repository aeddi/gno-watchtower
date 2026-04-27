# validator.vote_cast — Validator signed and submitted a vote

**Source signal type:** log (Loki)
**Gnoland versions:** v0.x onward

## What it represents

A validator signed and broadcast a prevote, precommit, or proposal to the
network. One event is emitted per signing action, carrying the height, round,
and vote type.

## How it's detected

The VoteCast handler matches gnoland slog lines whose `msg` field contains
either `"Signed proposal"` or `"Signed and pushed vote"`. The `height` and
`round` fields are extracted from the JSON log line. The `type` field provides
the vote type when present; for proposal lines the type is inferred as
`"Proposal"`. The `validator` label from the Loki stream identifies the node.

## Linked source code

- gnoland: `tm2/pkg/bft/consensus/state.go` (`signVote`, `signProposal`)

## Payload fields

- `height` (int64) — block height of the vote
- `round` (int32) — consensus round number
- `vote_type` (string) — one of `Prevote`, `Precommit`, or `Proposal`
