# chain.valset_changed — Validator set updated

**Source signal type:** log (Loki)
**Gnoland versions:** v0.x onward

## What it represents

The active validator set was modified: one or more validators were added or
removed. Currently only the addition path is populated from log data;
removals are left empty pending Phase-14 replay test validation.

## How it's detected

The ValsetChanged handler matches gnoland slog lines whose `msg` field
contains `"Updates to validators"`. The `height` field records the block at
which the change occurred. The `updates` field is an array of objects each
with `address` and `power` keys, which are mapped to `ValsetMember` entries
in the `added` list. The subject is set to `types.SubjectChain`.

## Linked source code

- gnoland: `tm2/pkg/bft/state/execution.go` (`updateValidators`)

## Payload fields

- `height` (int64) — block height at which the valset changed
- `added` ([]ValsetMember) — validators added to the set, each with `address` and `voting_power`
- `removed` ([]ValsetMember) — validators removed (always nil until phase-14 data is available)
