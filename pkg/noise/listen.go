package noise

import (
	"fmt"
	"net"
	"sync/atomic"
	"time"
)

// noDeadline is used to clear a previously-set deadline on a net.Conn.
var noDeadline = time.Time{}

// acceptResultBuffer is the small pre-handshake pipeline that decouples the
// accept path from the handshake path. Ready Noise Conns (or transient
// net.Listener errors) pile up here to be consumed by Accept. A value of 16
// absorbs bursts of fast handshakes without letting a slow consumer starve
// the accept loop — if this channel fills, the accept loop backpressures
// naturally because handshake goroutines block on the send.
const acceptResultBuffer = 16

// Listen binds a TCP listener on addr and wraps every inbound connection in a
// Noise-XX responder handshake. Failed handshakes (wrong authorized key, bad
// framing, slow clients timing out, …) are logged to onReject and the raw
// connection is closed — they never surface via Accept.
//
// handshakeTimeout bounds the handshake phase; set to a few seconds to keep
// slow-loris attackers from tying up file descriptors. A zero value disables
// the timeout (not recommended).
//
// Accept never blocks behind a single slow peer: each inbound TCP connection
// is handshaken in its own goroutine, and ready Conns are queued to the caller
// through an internal buffered channel. This mirrors crypto/tls.Listener's
// behavior and prevents a single stalled peer from denying service to the
// whole fleet.
type Listen struct {
	raw              net.Listener
	cfg              Config
	handshakeTimeout time.Duration
	onReject         func(remote net.Addr, err error)

	results chan acceptResult
	stopCh  chan struct{}
	closed  atomic.Bool
}

type acceptResult struct {
	conn net.Conn
	err  error
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
	l := &Listen{
		raw:              lis,
		cfg:              cfg,
		handshakeTimeout: handshakeTimeout,
		onReject:         onReject,
		results:          make(chan acceptResult, acceptResultBuffer),
		stopCh:           make(chan struct{}),
	}
	go l.loop()
	return l, nil
}

// loop runs the TCP accept loop and dispatches handshakes to per-connection
// goroutines. Terminates when Close is called or the underlying listener
// returns a permanent error.
func (l *Listen) loop() {
	for {
		raw, err := l.raw.Accept()
		if err != nil {
			if l.closed.Load() {
				return
			}
			// Surface the error to the next Accept caller, then exit — a
			// listener.Accept that's not due to Close is not recoverable.
			select {
			case l.results <- acceptResult{err: err}:
			case <-l.stopCh:
			}
			return
		}
		go l.handshake(raw)
	}
}

// handshake runs the Noise responder handshake against raw in its own
// goroutine so a slow peer cannot stall the accept loop. On success the
// resulting Conn is published to results; on failure raw is closed and the
// reject hook is notified.
func (l *Listen) handshake(raw net.Conn) {
	if l.handshakeTimeout > 0 {
		_ = raw.SetDeadline(time.Now().Add(l.handshakeTimeout))
	}
	conn, err := runHandshake(raw, l.cfg, false)
	if err != nil {
		if l.onReject != nil {
			l.onReject(raw.RemoteAddr(), err)
		}
		raw.Close()
		return
	}
	_ = raw.SetDeadline(noDeadline)
	select {
	case l.results <- acceptResult{conn: conn}:
	case <-l.stopCh:
		// Listener closed before anyone could consume this conn.
		conn.Close()
	}
}

// Accept blocks until a successfully-handshaken Conn is available, or the
// listener is closed, or the underlying accept loop surfaced an error.
func (l *Listen) Accept() (net.Conn, error) {
	select {
	case r, ok := <-l.results:
		if !ok {
			return nil, net.ErrClosed
		}
		if r.err != nil {
			return nil, r.err
		}
		return r.conn, nil
	case <-l.stopCh:
		return nil, net.ErrClosed
	}
}

// Close stops accepting connections. Does not close already-accepted Conns.
// In-flight handshakes observe the close via stopCh and drop their results.
func (l *Listen) Close() error {
	if !l.closed.CompareAndSwap(false, true) {
		return nil
	}
	close(l.stopCh)
	return l.raw.Close()
}

// Addr returns the underlying listener's address.
func (l *Listen) Addr() net.Addr { return l.raw.Addr() }

// Compile-time confirmation that Listen satisfies net.Listener.
var _ net.Listener = (*Listen)(nil)
