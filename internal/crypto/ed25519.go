// Package crypto wraps the stdlib Ed25519 verifier with helpers that match the
// signing payload conventions used by tweetnacl-js on the client side.
package crypto

import (
	"crypto/ed25519"
	"encoding/base64"
)

// VerifyREST returns true if `signatureB64` is a valid Ed25519 signature of
// (rawBodyStr + ":" + timestamp) by `publicKeyB64`. For HTTP method GET,
// rawBodyStr is treated as an empty string regardless of the supplied value
// (matching the JS client's `req.method === 'GET' ? '' : JSON.stringify(req.body)`).
func VerifyREST(publicKeyB64, signatureB64, rawBodyStr, timestamp, method string) bool {
	pub, err := base64.StdEncoding.DecodeString(publicKeyB64)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return false
	}
	sig, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return false
	}
	body := rawBodyStr
	if method == "GET" {
		body = ""
	}
	msg := []byte(body + ":" + timestamp)
	return ed25519.Verify(pub, msg, sig)
}

// VerifyTimestamp returns true if `signatureB64` is a valid Ed25519 signature
// of `timestamp` by `publicKeyB64`. Used by the Socket.IO handshake auth.
func VerifyTimestamp(publicKeyB64, signatureB64, timestamp string) bool {
	pub, err := base64.StdEncoding.DecodeString(publicKeyB64)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return false
	}
	sig, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(pub, []byte(timestamp), sig)
}
