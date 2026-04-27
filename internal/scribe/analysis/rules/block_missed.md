# block_missed_v1 — Validator missed a block

**Severity:** warning
**Gnoland versions:** v0.x onward

## What it detects

A specific validator did not produce a `validator.vote_cast` event for a
height that the chain has committed. One emission per (validator, height).

## Detection logic

On every `chain.block_committed{height=H}` event, the rule queries each
expected validator's last 50 `validator.vote_cast` events and emits a
`block_missed` diagnostic for any validator without a matching `height=H`
vote. No recovery semantics — each missed block is its own logical incident.

## Common causes

- Validator process restarted across the height boundary
- Validator's connectivity to its peers degraded transiently
- Validator's mempool was full and rejected the proposed block

## Remediation / workaround

1. Check the validator's logs at the relevant Loki link (below) for restarts
   or connection errors around the missed height.
2. Confirm the validator was running and reachable: query
   `up{validator="<id>"}` in VictoriaMetrics for the surrounding minute.
3. If multiple validators are missing the same height, escalate to a chain
   incident — `consensus_stuck_v1` or `bft_at_risk_v1` likely also fired.

## Linked signals

- Loki: `{validator="$validator"} |~ "received complete proposal block"`
- VictoriaMetrics: `up{validator="$validator"}`
- Source code: `gnoland/tm2/pkg/bft/consensus/state.go` (vote dispatch)

## Example incident

Not yet recorded.
