package noise

import (
	"bytes"
	"fmt"

	"github.com/flynn/noise"
)

// cipherSuite is the Noise cipher suite used for every connection:
// Curve25519 DH, ChaCha20-Poly1305 AEAD, BLAKE2b hash. Matches WireGuard's
// choice and the defaults used by libp2p-noise, so there's no negotiation and
// no room for downgrade.
var cipherSuite = noise.NewCipherSuite(noise.DH25519, noise.CipherChaChaPoly, noise.HashBLAKE2b)

// prologue is mixed into the handshake hash on both sides. Treating it as a
// version identifier so future protocol tweaks can't accidentally interop with
// old peers without both sides upgrading.
var prologue = []byte("gno-watchtower-noise-v1")

// Config configures either side of a Noise-XX session.
//
// Static is mandatory — both sides always carry a static identity, even when
// they don't verify the peer's identity.
//
// AuthorizedKeys is optional: if nil, any peer is accepted after a successful
// handshake (confidentiality-only); if non-empty, the peer's static public key
// must appear in the list or the connection is closed after the handshake.
type Config struct {
	Static         Keypair
	AuthorizedKeys [][]byte // optional; exact-match check on peer static pubkey
}

// Clone returns a copy safe to mutate.
func (c Config) Clone() Config {
	out := Config{Static: c.Static}
	if c.AuthorizedKeys != nil {
		out.AuthorizedKeys = make([][]byte, len(c.AuthorizedKeys))
		for i, k := range c.AuthorizedKeys {
			out.AuthorizedKeys[i] = append([]byte(nil), k...)
		}
	}
	return out
}

// authorizePeer returns an error if the peer's static key isn't acceptable.
// Called after the handshake completes. With no AuthorizedKeys set we accept
// anything (the handshake itself guarantees encryption and the peer's
// possession of the private half of whatever static key it sent).
func (c Config) authorizePeer(peerStatic []byte) error {
	if len(c.AuthorizedKeys) == 0 {
		return nil
	}
	for _, k := range c.AuthorizedKeys {
		if bytes.Equal(k, peerStatic) {
			return nil
		}
	}
	return fmt.Errorf("peer static key %x not in authorized_keys", peerStatic)
}

// validate ensures the Config can produce a handshake state.
func (c Config) validate() error {
	if len(c.Static.Private) != 32 {
		return fmt.Errorf("static private key: got %d bytes, want 32", len(c.Static.Private))
	}
	if len(c.Static.Public) != 32 {
		return fmt.Errorf("static public key: got %d bytes, want 32", len(c.Static.Public))
	}
	for i, k := range c.AuthorizedKeys {
		if len(k) != 32 {
			return fmt.Errorf("authorized_keys[%d]: got %d bytes, want 32", i, len(k))
		}
	}
	return nil
}
