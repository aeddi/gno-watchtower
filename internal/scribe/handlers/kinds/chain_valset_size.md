# chain.valset_size — Chain validator set size

**Source signal type:** metric (VictoriaMetrics)
**Gnoland versions:** v0.x onward

## What it represents

The total number of validators in the active set and their combined voting
power, aggregated per poll tick. This handler only upserts `samples_chain`
rows; it does not emit events.

## How it's detected

The ValsetSize handler matches the `sentinel_rpc_validator_set_power` metric,
which carries one observation per active validator per poll. The handler
groups observations by their `Metric.Time` timestamp. When a new tick arrives
(i.e. a timestamp strictly later than what has been seen), it flushes the
prior tick's aggregate as a single `samples_chain` upsert with `valset_size`
set to the count of observations and `total_voting_power` set to their sum.

## Linked source code

- gnoland: `tm2/pkg/bft/types/validator_set.go` (validator set power)
- sentinel: `internal/sentinel/collector/rpc.go` (RPC validator-set scrape)

## Payload fields

No event payload — this handler only produces sample upserts:

- `valset_size` (int16) — number of active validators in the set
- `total_voting_power` (int64) — sum of all validators' voting power
