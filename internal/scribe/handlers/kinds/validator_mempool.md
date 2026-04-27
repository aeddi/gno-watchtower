# validator.mempool — Validator mempool transaction count

**Source signal type:** metric (VictoriaMetrics)
**Gnoland versions:** v0.x onward

## What it represents

The number of transactions currently pending in a validator's mempool. This
handler only upserts `samples_validator` rows; it does not emit events.

## How it's detected

The Mempool handler matches the `sentinel_rpc_mempool_txs` metric scraped
from the gnoland RPC endpoint by the sentinel collector. On each observation
the handler upserts the `mempool_txs` column in `samples_validator` with a
4-microsecond offset to avoid primary-key collisions with other handlers
writing at the same metric timestamp.

## Linked source code

- gnoland: `tm2/pkg/bft/mempool/reactor.go` (mempool size gauge)
- sentinel: `internal/sentinel/collector/rpc.go` (RPC mempool scrape)

## Payload fields

No event payload — this handler only produces sample upserts:

- `mempool_txs` (int32) — number of pending transactions in the mempool
