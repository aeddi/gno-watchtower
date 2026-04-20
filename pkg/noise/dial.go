package noise

import (
	"context"
	"fmt"
	"net"
)

// Dial opens a TCP connection to addr and performs the initiator side of a
// Noise-XX handshake. The returned Conn is safe for HTTP or any protocol
// expecting a net.Conn.
//
// ctx cancellation cancels an in-progress dial; once the handshake completes
// the ctx is no longer consulted (use Conn.SetDeadline for that).
func Dial(ctx context.Context, network, addr string, cfg Config) (*Conn, error) {
	if network == "" {
		network = "tcp"
	}
	var d net.Dialer
	raw, err := d.DialContext(ctx, network, addr)
	if err != nil {
		return nil, fmt.Errorf("tcp dial %s: %w", addr, err)
	}
	// Propagate ctx deadline to the handshake.
	if dl, ok := ctx.Deadline(); ok {
		_ = raw.SetDeadline(dl)
	}
	conn, err := runHandshake(raw, cfg, true)
	if err != nil {
		raw.Close()
		return nil, fmt.Errorf("noise handshake to %s: %w", addr, err)
	}
	// Clear the dial-time deadline so user calls can set their own.
	_ = raw.SetDeadline(noDeadline)
	return conn, nil
}
