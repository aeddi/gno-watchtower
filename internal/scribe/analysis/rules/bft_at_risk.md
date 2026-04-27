# bft_at_risk_v1 — BFT at risk

**Severity:** error
**Gnoland versions:** v0.x onward

## What it detects

The fraction of online validators is below the BFT-safety threshold
(default 33.33% offline = 2/3 online). The chain is at risk of halting if
even one more validator goes offline. Auto-recovers when validators come
back online.

## Detection logic

On every `validator.went_offline` or `validator.came_online` event, the
rule reads the latest `samples_chain` row and computes
`(valset_size - online_count) / valset_size * 100`. If the offline fraction
≥ `voting_power_threshold_pct` (default 33.33), emit `state=open`. When the
fraction returns below the threshold, emit `state=recovered`.

## Common causes

- Multiple validators restarting concurrently (rolling deploy without
  coordination)
- Network partition between validator hosts
- Sentinel collector dropped a heartbeat — verify against raw RPC
  reachability before declaring an incident

## Remediation / workaround

1. List currently-offline validators by querying scribe:
   `/api/events?kind=validator.went_offline&state=open`.
2. Check each offline validator's `up{}` series in VictoriaMetrics for the
   cause of the dropout.
3. If the cause is a sentinel scrape gap (false positive), suppress noise
   by tightening sentinel's scrape interval; the rule will auto-recover
   once the gap fills.
4. If genuinely offline: bring the validators back; if you can't, escalate
   to a chain incident.

## Linked signals

- Loki: `{level=~"warn|error"} |~ "consensus" |~ "validator"`
- VictoriaMetrics: `(count(up{job=~".*validator.*"} == 0)) / count(up{job=~".*validator.*"})`
- Source code: `internal/scribe/handlers/logs.go` (`Online` handler)

## Example incident

Not yet recorded.
