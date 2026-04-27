# validator.peer_connected — Validator established a peer connection

**Source signal type:** log (Loki)
**Gnoland versions:** v0.x onward

## What it represents

A new outbound peer connection was successfully added at a validator node.
One event is emitted per connection, carrying the full peer address and the
derived peer ID.

## How it's detected

The PeerConnected handler matches gnoland slog lines whose `msg` field
contains `"Added peer"`. The `peer` field from the log line is extracted; the
peer ID is derived as the portion before the `@` character (the node ID in
`<id>@<host>:<port>` format). The `validator` label from the Loki stream
identifies the node. The direction is always `"out"` since gnoland only logs
the `Added peer` message for outbound connections.

## Linked source code

- gnoland: `tm2/pkg/p2p/switch.go` (`addPeer`)

## Payload fields

- `peer` (string) — full peer address (`<id>@<host>:<port>`)
- `peer_id` (string) — node ID portion of the peer address
- `direction` (string) — always `"out"` (outbound connection)
