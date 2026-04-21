// Package noise wraps github.com/flynn/noise to provide an authenticated,
// encrypted, framed net.Conn between sentinel and beacon.
//
// Handshake pattern is Noise-XX: both sides exchange static public keys
// during the handshake. After the handshake each side can optionally verify
// the peer's static public key against a local list; if no list is set the
// session is accepted (confidentiality-only mode).
//
// Cipher suite is fixed: Curve25519 for DH, ChaCha20-Poly1305 for AEAD,
// BLAKE2b for hashing. Same choice as WireGuard.
package noise

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/flynn/noise"
)

// Filenames inside a keys directory. Privkey holds 32 raw bytes of secret key
// material hex-encoded; pubkey holds 32 raw bytes of the public key. Two files
// so operators can chmod the private one tighter (0600) than the public one.
const (
	PrivKeyFilename = "privkey"
	PubKeyFilename  = "pubkey"
)

// Keypair is a Curve25519 keypair used as the static identity for a Noise
// endpoint. Hex-encoded on disk, raw in memory.
type Keypair noise.DHKey

// GenerateKeypair creates a fresh Curve25519 keypair using Noise's RNG.
func GenerateKeypair() (Keypair, error) {
	kp, err := noise.DH25519.GenerateKeypair(nil) // nil → crypto/rand
	if err != nil {
		return Keypair{}, fmt.Errorf("generate keypair: %w", err)
	}
	return Keypair(kp), nil
}

// WriteKeypair writes the keypair into dir as two files:
//
//	<dir>/privkey — mode 0600, 64 hex characters of the private key
//	<dir>/pubkey  — mode 0644, 64 hex characters of the public key
//
// Overwrites existing files. Returns an error if dir doesn't exist or can't be
// written.
func WriteKeypair(dir string, kp Keypair) error {
	if len(kp.Private) != 32 || len(kp.Public) != 32 {
		return fmt.Errorf("keypair: bad sizes (priv=%d pub=%d want 32 each)", len(kp.Private), len(kp.Public))
	}
	priv := hex.EncodeToString(kp.Private) + "\n"
	pub := hex.EncodeToString(kp.Public) + "\n"
	privPath := filepath.Join(dir, PrivKeyFilename)
	pubPath := filepath.Join(dir, PubKeyFilename)
	if err := os.WriteFile(privPath, []byte(priv), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", privPath, err)
	}
	if err := os.WriteFile(pubPath, []byte(pub), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", pubPath, err)
	}
	return nil
}

// LoadKeypair reads a keypair from the two files in dir.
func LoadKeypair(dir string) (Keypair, error) {
	privPath := filepath.Join(dir, PrivKeyFilename)
	pubPath := filepath.Join(dir, PubKeyFilename)
	priv, err := loadHexFile(privPath, 32)
	if err != nil {
		return Keypair{}, fmt.Errorf("read private key: %w", err)
	}
	pub, err := loadHexFile(pubPath, 32)
	if err != nil {
		return Keypair{}, fmt.Errorf("read public key: %w", err)
	}
	return Keypair{Private: priv, Public: pub}, nil
}

// DecodePublicKey parses a 64-char hex string into a 32-byte public key.
// Accepts surrounding whitespace so operators can paste values that picked up
// a trailing newline.
func DecodePublicKey(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	key, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("public key hex: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("public key: got %d bytes, want 32", len(key))
	}
	return key, nil
}

func loadHexFile(path string, wantLen int) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s := strings.TrimSpace(string(b))
	decoded, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("hex decode %s: %w", path, err)
	}
	if len(decoded) != wantLen {
		return nil, fmt.Errorf("%s: got %d bytes, want %d", path, len(decoded), wantLen)
	}
	return decoded, nil
}
