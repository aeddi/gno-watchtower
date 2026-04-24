package gpub_test

import (
	"strings"
	"testing"

	"github.com/aeddi/gno-watchtower/pkg/gpub"
)

// TestEncodeEd25519FromBase64_MatchesGnolandSecretsGet pins the encoder to a
// vector produced by `gnoland secrets get validator_key.pub_key -raw
// --data-dir <gno-cluster>/internal/secrets/node-1`. If this ever drifts,
// gnoland changed its pubkey wire format — not something to paper over.
func TestEncodeEd25519FromBase64_MatchesGnolandSecretsGet(t *testing.T) {
	const b64 = "mKsg1XPxANeixURll0tm+FdymdT7qyOMs8h0lliCK6w="
	const want = "gpub1pggj7ard9eg82cjtv4u52epjx56nzwgjyg9zpx9tyr2h8ugq673v23r9ja9kd7zhw2vaf7atywxt8jr5jevgy2avyp9krz"

	got, err := gpub.EncodeEd25519FromBase64(b64)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestEncodeEd25519FromBase64_RejectsWrongLength(t *testing.T) {
	// 16-byte payload — too short to be ed25519.
	_, err := gpub.EncodeEd25519FromBase64("AAECAwQFBgcICQoLDA0ODw==")
	if err == nil || !strings.Contains(err.Error(), "32") {
		t.Errorf("want 32-byte length error, got %v", err)
	}
}

func TestEncodeEd25519FromBase64_RejectsInvalidBase64(t *testing.T) {
	_, err := gpub.EncodeEd25519FromBase64("not-base64!!")
	if err == nil {
		t.Error("expected base64 decode error, got nil")
	}
}
