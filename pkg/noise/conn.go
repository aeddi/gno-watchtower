package noise

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/flynn/noise"
)

// Frame format on the wire:
//
//	[ 2-byte big-endian length ][ ciphertext ... ]
//
// The 2-byte length matches Noise's own message-size limit (65535 bytes per
// message). Payloads larger than the Noise AEAD plaintext limit are split
// across multiple frames inside Write; reads reassemble on demand so io.Reader
// semantics are preserved.
const (
	// maxNoisePlaintext is the largest plaintext that fits in one Noise frame:
	// Noise limits ciphertext to 65535, and ChaCha20-Poly1305 adds a 16-byte
	// authentication tag. Leave another few bytes of slack for safety.
	maxNoisePlaintext = 65535 - 16 - 16
)

// Conn is a Noise-wrapped net.Conn. Construct via Dial or (post-handshake) via
// the Listener's Accept. Safe for concurrent Read and Write, but not safe for
// concurrent Read-from-two-goroutines or Write-from-two-goroutines.
type Conn struct {
	raw net.Conn

	// post-handshake peer static pubkey (for callers that want to inspect)
	peerStatic []byte

	// cipherstates produced by the handshake
	sendCS *noise.CipherState
	recvCS *noise.CipherState

	// read buffer for plaintext spilling across Read calls
	readMu  sync.Mutex
	readBuf []byte

	// serialize writes so a second goroutine doesn't interleave a single Write
	writeMu sync.Mutex
}

// PeerStatic returns the peer's 32-byte static public key.
func (c *Conn) PeerStatic() []byte {
	out := make([]byte, len(c.peerStatic))
	copy(out, c.peerStatic)
	return out
}

// Read decrypts and returns plaintext.
func (c *Conn) Read(p []byte) (int, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	// Drain any plaintext left from the previous frame first.
	if len(c.readBuf) > 0 {
		n := copy(p, c.readBuf)
		c.readBuf = c.readBuf[n:]
		return n, nil
	}

	frame, err := readFrame(c.raw)
	if err != nil {
		return 0, err
	}
	plaintext, err := c.recvCS.Decrypt(nil, nil, frame)
	if err != nil {
		return 0, fmt.Errorf("noise decrypt: %w", err)
	}
	n := copy(p, plaintext)
	if n < len(plaintext) {
		c.readBuf = append(c.readBuf[:0], plaintext[n:]...)
	}
	return n, nil
}

// Write encrypts and sends p, framing into multiple Noise messages when
// necessary. Either all of p is sent or an error is returned.
func (c *Conn) Write(p []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	total := 0
	for len(p) > 0 {
		chunk := p
		if len(chunk) > maxNoisePlaintext {
			chunk = chunk[:maxNoisePlaintext]
		}
		ct, err := c.sendCS.Encrypt(nil, nil, chunk)
		if err != nil {
			return total, fmt.Errorf("noise encrypt: %w", err)
		}
		if err := writeFrame(c.raw, ct); err != nil {
			return total, err
		}
		p = p[len(chunk):]
		total += len(chunk)
	}
	return total, nil
}

// Close closes the underlying transport.
func (c *Conn) Close() error                       { return c.raw.Close() }
func (c *Conn) LocalAddr() net.Addr                { return c.raw.LocalAddr() }
func (c *Conn) RemoteAddr() net.Addr               { return c.raw.RemoteAddr() }
func (c *Conn) SetDeadline(t time.Time) error      { return c.raw.SetDeadline(t) }
func (c *Conn) SetReadDeadline(t time.Time) error  { return c.raw.SetReadDeadline(t) }
func (c *Conn) SetWriteDeadline(t time.Time) error { return c.raw.SetWriteDeadline(t) }

// readFrame reads a single [length][ciphertext] frame from r.
func readFrame(r io.Reader) ([]byte, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint16(hdr[:])
	if n == 0 {
		return nil, errors.New("noise: zero-length frame")
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// writeFrame writes a single [length][ciphertext] frame to w.
func writeFrame(w io.Writer, payload []byte) error {
	if len(payload) > 65535 {
		return fmt.Errorf("noise: frame too large (%d > 65535)", len(payload))
	}
	var hdr [2]byte
	binary.BigEndian.PutUint16(hdr[:], uint16(len(payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	if _, err := w.Write(payload); err != nil {
		return err
	}
	return nil
}
