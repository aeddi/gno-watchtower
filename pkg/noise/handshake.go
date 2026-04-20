package noise

import (
	"errors"
	"fmt"
	"net"

	"github.com/flynn/noise"
)

// runHandshake performs the full Noise-XX handshake over raw. Initiator sends
// first. Returns an authenticated Conn on success.
//
// Pattern XX message flow:
//
//	→ e                          (msg 1, initiator → responder, unencrypted)
//	← e, ee, s, es               (msg 2, responder → initiator, responder's static encrypted)
//	→ s, se                      (msg 3, initiator → responder, initiator's static encrypted)
//
// After msg 3 both sides call Split() to get their send/receive cipherstates.
func runHandshake(raw net.Conn, cfg Config, initiator bool) (*Conn, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	hsCfg := noise.Config{
		CipherSuite:   cipherSuite,
		Pattern:       noise.HandshakeXX,
		Initiator:     initiator,
		Prologue:      prologue,
		StaticKeypair: noise.DHKey(cfg.Static),
	}
	hs, err := noise.NewHandshakeState(hsCfg)
	if err != nil {
		return nil, fmt.Errorf("noise handshake state: %w", err)
	}

	// A tiny state machine: alternate writes/reads until Split() returns non-nil.
	// Initiator writes msg 1 + 3; responder writes msg 2.
	for i := 0; ; i++ {
		writeTurn := initiator == (i%2 == 0)
		if writeTurn {
			buf, cs1, cs2, err := hs.WriteMessage(nil, nil)
			if err != nil {
				return nil, fmt.Errorf("noise handshake write msg %d: %w", i+1, err)
			}
			if err := writeFrame(raw, buf); err != nil {
				return nil, fmt.Errorf("noise handshake send msg %d: %w", i+1, err)
			}
			if cs1 != nil {
				return finish(raw, hs, cs1, cs2, initiator, cfg)
			}
		} else {
			frame, err := readFrame(raw)
			if err != nil {
				return nil, fmt.Errorf("noise handshake recv msg %d: %w", i+1, err)
			}
			_, cs1, cs2, err := hs.ReadMessage(nil, frame)
			if err != nil {
				return nil, fmt.Errorf("noise handshake read msg %d: %w", i+1, err)
			}
			if cs1 != nil {
				return finish(raw, hs, cs1, cs2, initiator, cfg)
			}
		}
	}
}

// finish assembles the Conn after a successful handshake. Initiator uses cs1
// to encrypt and cs2 to decrypt (and responder the other way around) — that's
// Noise's Split() convention.
func finish(raw net.Conn, hs *noise.HandshakeState, cs1, cs2 *noise.CipherState, initiator bool, cfg Config) (*Conn, error) {
	peer := hs.PeerStatic()
	if len(peer) == 0 {
		return nil, errors.New("noise handshake completed without peer static key")
	}
	if err := cfg.authorizePeer(peer); err != nil {
		return nil, err
	}
	send, recv := cs1, cs2
	if !initiator {
		send, recv = cs2, cs1
	}
	return &Conn{
		raw:        raw,
		peerStatic: append([]byte(nil), peer...),
		sendCS:     send,
		recvCS:     recv,
	}, nil
}

// Ensure errors package stays imported even if callers rearrange usage.
var _ = errors.New
