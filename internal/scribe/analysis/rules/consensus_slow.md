# consensus_slow_v1 — Block production is slow

**Severity:** warning
**Gnoland versions:** v0.x onward

## What it detects

The p95 of inter-block durations over the last 50 commits exceeds the
configured `slow_threshold_seconds` (default 5s). Auto-recovers when the
window's p95 returns below the threshold.

## Detection logic

On every `chain.block_committed` event, the rule appends the gap from the
previous commit to a per-cluster ring buffer (capacity 50). When the buffer
has at least 5 samples, it computes p95 and compares to the threshold. Open
keyed by cluster.

## Common causes

- Validator load spike (high mempool, large blocks)
- Network latency between validator hosts
- Slow disk on a leader validator
- Determinism issue causing repeated round restarts (`consensus_stuck_v1`
  may also fire)

## Remediation / workaround

1. Look at the chain dashboard's "block time" panel — does the slowdown
   correlate with a load increase?
2. Inspect per-validator CPU / disk / mempool metrics in VictoriaMetrics
   for the same window.
3. If no obvious cause, sample consensus logs around a slow height in Loki.

## Linked signals

- Loki: `{level=~"info|warn"} |~ "received complete proposal block" |~ "height=$height"`
- VictoriaMetrics: `histogram_quantile(0.95, rate(scribe_writer_batch_duration_seconds_bucket[5m]))`
- Source code: `gnoland/tm2/pkg/bft/consensus/state.go`

## Example incident

Not yet recorded.
