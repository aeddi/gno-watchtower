# validator.peer_disconnected — Validator peer connection dropped

**Source signal type:** log (Loki)
**Gnoland versions:** v0.x onward

## What it represents

A peer connection at a validator node was terminated due to an error. One
event is emitted per disconnection, carrying the peer ID and the error reason.

## How it's detected

The PeerDisconnected handler matches gnoland slog lines whose `msg` field
contains `"Stopping peer for error"`. The `peer` field is parsed to extract
the node ID (the portion before `@`), and the `err` field provides the
disconnect reason. The `validator` label from the Loki stream identifies the
node.

## Linked source code

- gnoland: `tm2/pkg/p2p/switch.go` (`stopAndRemovePeer`)

## Payload fields

- `peer_id` (string) — node ID of the disconnected peer
- `reason` (string) — error message that caused the disconnection
