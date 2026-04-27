# consensus_stuck_v1 — Chain not advancing

**Severity:** error
**Gnoland versions:** v0.x onward

## What it detects

The most recent `chain.block_committed` event is older than
`threshold_seconds` (default 60s). The chain is not advancing. Auto-recovers
when a new block lands.

## Detection logic

Tick rule (15s eval cadence). Each tick, the rule queries the latest
`chain.block_committed` event from the store and compares its timestamp to
`now`. If older than `threshold_seconds`, emit `state=open` keyed by cluster.
On the next tick where a fresh block exists, emit `state=recovered`.

## Common causes

- Insufficient online validators to reach BFT (`bft_at_risk_v1` likely also
  fired)
- Network partition between validator hosts
- A validator is non-deterministic and consensus rounds are deadlocked

## Remediation / workaround

1. Confirm the chain is actually stuck: query `/api/state?subject=_chain` —
   does `block_height` match the latest committed event's height?
2. If `bft_at_risk_v1` is open: address that first; chain will likely
   resume on its own.
3. If quorum is fine but consensus is still stuck: capture a consensus
   round-step trace from each validator's logs, then escalate (likely a
   determinism break or a config divergence — operator intervention
   required).

## Linked signals

- Loki: `{level=~"warn|error"} |~ "consensus" |~ "round"`
- VictoriaMetrics: `time() - max(scribe_events_written_total{kind="chain.block_committed"}) BY (cluster_id) / 1`
- Source code: `gnoland/tm2/pkg/bft/consensus/state.go`

## Example incident

Not yet recorded.
