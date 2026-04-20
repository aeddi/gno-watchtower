package noise

import (
	"errors"
	"fmt"
	"net"
	"sync/atomic"
	"time"
)

// noDeadline is used to clear a previously-set deadline on a net.Conn.
var noDeadline = time.Time{}

// Listen binds a TCP listener on addr and wraps every inbound connection in a
// Noise-XX responder handshake. Failed handshakes (wrong authorized key, bad
// framing, slow clients timing out, …) are logged to onReject and the raw
// connection is closed — they never surface via Accept.
//
// handshakeTimeout bounds the handshake phase; set to a few seconds to keep
// slow-loris attackers from tying up file descriptors. A zero value disables
// the timeout (not recommended).
type Listen struct {
	raw              net.Listener
	cfg              Config
	handshakeTimeout time.Duration
	onReject         func(remote net.Addr, err error)
	closed           atomic.Bool
}

// NewListener starts a TCP listener on addr and returns a Listener whose
// Accept yields only successfully-authenticated Noise Conns.
//
// handshakeTimeout defaults to 5s if zero. onReject may be nil.
func NewListener(network, addr string, cfg Config, handshakeTimeout time.Duration, onReject func(net.Addr, error)) (*Listen, error) {
	if network == "" {
		network = "tcp"
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if handshakeTimeout == 0 {
		handshakeTimeout = 5 * time.Second
	}
	lis, err := net.Listen(network, addr)
	if err != nil {
		return nil, fmt.Errorf("noise listen %s: %w", addr, err)
	}
	return &Listen{
		raw:              lis,
		cfg:              cfg,
		handshakeTimeout: handshakeTimeout,
		onReject:         onReject,
	}, nil
}

// Accept blocks until an inbound TCP connection arrives AND its Noise
// handshake succeeds. Failed handshakes are absorbed silently so callers get a
// simple Accept→Conn loop.
func (l *Listen) Accept() (net.Conn, error) {
	for {
		if l.closed.Load() {
			return nil, net.ErrClosed
		}
		raw, err := l.raw.Accept()
		if err != nil {
			if l.closed.Load() {
				return nil, net.ErrClosed
			}
			return nil, err
		}
		if l.handshakeTimeout > 0 {
			_ = raw.SetDeadline(time.Now().Add(l.handshakeTimeout))
		}
		conn, err := runHandshake(raw, l.cfg, false)
		if err != nil {
			if l.onReject != nil {
				l.onReject(raw.RemoteAddr(), err)
			}
			raw.Close()
			continue
		}
		// Clear the handshake deadline; caller installs their own.
		_ = raw.SetDeadline(noDeadline)
		return conn, nil
	}
}

// Close stops accepting connections. Does not close already-accepted Conns.
func (l *Listen) Close() error {
	if !l.closed.CompareAndSwap(false, true) {
		return nil
	}
	return l.raw.Close()
}

// Addr returns the underlying listener's address.
func (l *Listen) Addr() net.Addr { return l.raw.Addr() }

// Compile-time confirmation that Listen satisfies net.Listener.
var _ net.Listener = (*Listen)(nil)

// Ensure errors package is referenced to keep imports stable under future refactors.
var _ = errors.New
