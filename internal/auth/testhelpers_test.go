package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func freshKeypair(t *testing.T) (string, string) {
	t.Helper()
	pub, sec, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(pub), base64.StdEncoding.EncodeToString(sec)
}

func signMessage(t *testing.T, secB64, msg string) string {
	t.Helper()
	sec, err := base64.StdEncoding.DecodeString(secB64)
	if err != nil {
		t.Fatal(err)
	}
	sig := ed25519.Sign(ed25519.PrivateKey(sec), []byte(msg))
	return base64.StdEncoding.EncodeToString(sig)
}
