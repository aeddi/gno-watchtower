package noise_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aeddi/gno-watchtower/pkg/noise"
)

func TestKeypair_WriteAndLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	orig, err := noise.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	if err := noise.WriteKeypair(dir, orig); err != nil {
		t.Fatal(err)
	}

	loaded, err := noise.LoadKeypair(dir)
	if err != nil {
		t.Fatal(err)
	}
	if string(loaded.Private) != string(orig.Private) {
		t.Error("private key mismatch after round-trip")
	}
	if string(loaded.Public) != string(orig.Public) {
		t.Error("public key mismatch after round-trip")
	}
}

func TestKeypair_WriteSetsExpectedFilePerms(t *testing.T) {
	dir := t.TempDir()
	kp, err := noise.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	if err := noise.WriteKeypair(dir, kp); err != nil {
		t.Fatal(err)
	}
	privInfo, err := os.Stat(filepath.Join(dir, noise.PrivKeyFilename))
	if err != nil {
		t.Fatal(err)
	}
	if privInfo.Mode().Perm() != 0o600 {
		t.Errorf("privkey perms: got %o, want 0600", privInfo.Mode().Perm())
	}
	pubInfo, err := os.Stat(filepath.Join(dir, noise.PubKeyFilename))
	if err != nil {
		t.Fatal(err)
	}
	if pubInfo.Mode().Perm() != 0o644 {
		t.Errorf("pubkey perms: got %o, want 0644", pubInfo.Mode().Perm())
	}
}

func TestDecodePublicKey_TrimsWhitespace(t *testing.T) {
	kp, err := noise.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	// Reuse LoadKeypair's hex rendering via manual hex encoding. We just need
	// a valid 64-char hex string; any will do.
	raw, err := os.ReadFile(writeHex(t, kp.Public))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := noise.DecodePublicKey(" " + strings.TrimSpace(string(raw)) + "\n"); err != nil {
		t.Errorf("should trim whitespace, got: %v", err)
	}
}

func TestDecodePublicKey_RejectsWrongLength(t *testing.T) {
	if _, err := noise.DecodePublicKey("abcd"); err == nil {
		t.Error("expected error for short key")
	}
}

// writeHex writes hex-encoded bytes to a temp file, returns the path.
func writeHex(t *testing.T, b []byte) string {
	t.Helper()
	dir := t.TempDir()
	kp := noise.Keypair{Private: make([]byte, 32), Public: b}
	// WriteKeypair needs both halves; supply a zero private since we only
	// read the pub file from disk.
	if err := noise.WriteKeypair(dir, kp); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, noise.PubKeyFilename)
}
