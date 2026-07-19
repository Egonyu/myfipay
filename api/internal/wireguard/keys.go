// Package wireguard generates Curve25519 keypairs for the per-router
// management tunnel. Key format matches `wg genkey` / `wg pubkey`:
// 32 bytes, base64 (std encoding, with padding).
package wireguard

import (
	"crypto/rand"
	"encoding/base64"

	"golang.org/x/crypto/curve25519"
)

func GenerateKeypair() (privB64, pubB64 string, err error) {
	priv := make([]byte, curve25519.ScalarSize)
	if _, err = rand.Read(priv); err != nil {
		return "", "", err
	}
	// RFC 7748 clamping, same as wg genkey
	priv[0] &= 248
	priv[31] = (priv[31] & 127) | 64

	pub, err := curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(priv), base64.StdEncoding.EncodeToString(pub), nil
}
