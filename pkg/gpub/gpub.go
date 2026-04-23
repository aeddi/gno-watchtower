// Package gpub encodes a gnoland validator ed25519 public key in its
// canonical `gpub1...` bech32 form — the same string `gnoland secrets get
// validator_key.pub_key -raw` prints.
//
// Gnoland wraps the raw 32-byte ed25519 key in a protobuf Any envelope
// before bech32-encoding with the "gpub" HRP. For /tm.PubKeyEd25519 the
// wrapping is:
//
//	0a 11 /tm.PubKeyEd25519 12 22 0a 20 <32-byte pubkey>
//
// The encoder here only handles ed25519; secp256k1 is not emitted by any
// current gnoland validator and would need a different wrapping prefix.
package gpub

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// EncodeEd25519FromBase64 takes the base64-encoded raw 32-byte ed25519
// public key (as it appears in tendermint /validators and /genesis under
// `pub_key.value`) and returns its `gpub1...` bech32 canonical form.
func EncodeEd25519FromBase64(b64 string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("gpub: base64 decode: %w", err)
	}
	if len(raw) != 32 {
		return "", fmt.Errorf("gpub: expected 32-byte ed25519 key, got %d bytes", len(raw))
	}
	return encodeEd25519(raw), nil
}

func encodeEd25519(pk []byte) string {
	typeURL := []byte("/tm.PubKeyEd25519")
	wrapped := make([]byte, 0, 8+len(typeURL)+len(pk))
	wrapped = append(wrapped, 0x0a, byte(len(typeURL)))
	wrapped = append(wrapped, typeURL...)
	wrapped = append(wrapped, 0x12, 0x22, 0x0a, 0x20)
	wrapped = append(wrapped, pk...)
	return bech32Encode("gpub", wrapped)
}

// ---- BIP173 bech32 encoder

const charset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

var bech32Gen = [5]uint32{0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3}

func bech32Encode(hrp string, data []byte) string {
	d5 := convertBits(data, 8, 5, true)
	combined := append(d5, createChecksum(hrp, d5)...)
	var sb strings.Builder
	sb.Grow(len(hrp) + 1 + len(combined))
	sb.WriteString(hrp)
	sb.WriteByte('1')
	for _, b := range combined {
		sb.WriteByte(charset[b])
	}
	return sb.String()
}

// convertBits regroups bytes between base-2^fromBits and base-2^toBits. The
// 8→5 direction requires pad=true for the final partial group.
func convertBits(data []byte, fromBits, toBits uint, pad bool) []byte {
	var acc, bits uint
	maxv := uint(1)<<toBits - 1
	out := make([]byte, 0, (len(data)*int(fromBits)+int(toBits)-1)/int(toBits))
	for _, b := range data {
		acc = acc<<fromBits | uint(b)
		bits += fromBits
		for bits >= toBits {
			bits -= toBits
			out = append(out, byte(acc>>bits&maxv))
		}
	}
	if pad && bits > 0 {
		out = append(out, byte(acc<<(toBits-bits)&maxv))
	}
	return out
}

func polymod(values []byte) uint32 {
	chk := uint32(1)
	for _, v := range values {
		top := chk >> 25
		chk = (chk&0x1ffffff)<<5 ^ uint32(v)
		for i := 0; i < 5; i++ {
			if top>>i&1 == 1 {
				chk ^= bech32Gen[i]
			}
		}
	}
	return chk
}

func hrpExpand(hrp string) []byte {
	out := make([]byte, 2*len(hrp)+1)
	for i, c := range hrp {
		out[i] = byte(c) >> 5
		out[i+len(hrp)+1] = byte(c) & 31
	}
	return out
}

func createChecksum(hrp string, data []byte) []byte {
	values := append(hrpExpand(hrp), data...)
	values = append(values, 0, 0, 0, 0, 0, 0)
	pm := polymod(values) ^ 1
	out := make([]byte, 6)
	for i := 0; i < 6; i++ {
		out[i] = byte(pm >> uint(5*(5-i)) & 31)
	}
	return out
}
