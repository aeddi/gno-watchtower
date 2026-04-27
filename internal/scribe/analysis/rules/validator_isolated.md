# validator_isolated_v1 — Validator network-isolated

**Severity:** warning
**Gnoland versions:** v0.x onward

## What it detects

A validator's combined inbound + outbound peer count has been below
`min_peers` (default 2) for at least `isolated_threshold_seconds` (default
30s). Sustained — a single transient flap doesn't trigger.

## Detection logic

Subscribes to `validator.peer_connected` and `validator.peer_disconnected`,
plus a 30s tick. On each trigger, reads the validator's latest merged
samples (`GetMergedSampleValidator` over a 30s window) and checks the
combined peer count. Maintains a per-validator "first time peers went low"
timestamp; emits open only after the sustained threshold has elapsed. The
tick covers cases where peers stay low without any new peer events firing.

## Common causes

- Validator host's network egress is partitioned from the rest of the
  cluster
- Validator config has stale or unreachable seed/peer addresses
- Sentinel scrape is missing for this validator (false positive — verify
  with raw metrics)

## Remediation / workaround

1. Check the validator host's outbound connectivity to known peers
   (`telnet <peer>:26656`).
2. Inspect `inbound_peers_gauge` and `outbound_peers_gauge` for the
   validator in VictoriaMetrics; confirm the value matches what the rule
   sees.
3. If a config error: update peer/seed list and restart the validator.
4. If a network partition: address it; the rule recovers automatically
   once peers return above `min_peers`.

## Linked signals

- Loki: `{validator="$validator"} |= "peer"`
- VictoriaMetrics: `inbound_peers_gauge{validator="$validator"} + outbound_peers_gauge{validator="$validator"}`
- Source code: `internal/scribe/handlers/metrics.go` (`Peers` handler)

## Example incident

Not yet recorded.
