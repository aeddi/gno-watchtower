# validator.came_online — Validator online/offline transitions

**Source signal type:** metric (VictoriaMetrics)
**Gnoland versions:** v0.x onward

## What it represents

State transitions of a validator's reachability as observed by the sentinel
collector. The Online handler emits `validator.came_online` when the metric
transitions from 0 to 1 and `validator.went_offline` when it transitions from
1 to 0. `came_online` events carry the gap duration since the last seen time.

## How it's detected

The Online handler matches the `sentinel_validator_online` metric. A value
greater than 0 means the validator is reachable; 0 means unreachable. The
handler tracks the previous state per validator and fires an event only on
transitions, suppressing repeated same-state observations.

## Linked source code

- sentinel: `internal/sentinel/collector/rpc.go` (online probe)

## Payload fields

For `validator.came_online`:

- `gap_duration` (string) — duration since last observed online time

For `validator.went_offline`:

- `last_seen` (time.Time) — timestamp of the last online observation
