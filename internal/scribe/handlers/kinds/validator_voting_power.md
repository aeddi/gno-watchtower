# validator.voting_power — Validator voting power

**Source signal type:** metric (VictoriaMetrics)
**Gnoland versions:** v0.x onward

## What it represents

The current voting power assigned to a validator in the active validator set.
This handler only upserts `samples_validator` rows; it does not emit events.

## How it's detected

The VotingPower handler matches the `sentinel_rpc_validator_voting_power`
metric scraped from the gnoland RPC endpoint by the sentinel collector. On
each observation the handler upserts the `voting_power` column in
`samples_validator` with a 5-microsecond offset to avoid primary-key
collisions with other handlers writing at the same metric timestamp.

## Linked source code

- gnoland: `tm2/pkg/bft/types/validator.go` (voting power field)
- sentinel: `internal/sentinel/collector/rpc.go` (RPC validator-set scrape)

## Payload fields

No event payload — this handler only produces sample upserts:

- `voting_power` (int64) — current voting power of the validator
