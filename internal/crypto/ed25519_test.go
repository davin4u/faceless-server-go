package crypto

import "testing"

// Vectors generated from tweetnacl-js with seed = 0..31.
// Vector 1: signature of "hello:1700000000"
// Vector 2: signature of "1700000000"
const (
	vectorPubKey       = "A6EHv/POEL4dcN0Y50vAmWfk1jCbpQ1fHdyGZBJVMbg="
	vectorSigPostHello = "wBDd4RlVDXsxPpKjmR83x/AVT8vaO4aDY+lHoTvDRKoGo/Olq6i8ZHvnj/Jc68Ksru0Wys+S4ycGoQfDX7qhCA=="
	vectorSigTsOnly    = "WFikIfY+qTfpOiTLlXm9hu9losQpGzk213Y95OGyZs8seKxPPMw+pAcS2dI8eyniQwe345m+hZa7eEEiufpgCw=="
)

func TestVerifyREST_KnownVector(t *testing.T) {
	if !VerifyREST(vectorPubKey, vectorSigPostHello, "hello", "1700000000", "POST") {
		t.Fatal("expected valid POST signature to verify")
	}
}

func TestVerifyREST_TamperedBody(t *testing.T) {
	if VerifyREST(vectorPubKey, vectorSigPostHello, "tampered", "1700000000", "POST") {
		t.Error("tampered body should fail")
	}
}

func TestVerifyREST_TamperedTimestamp(t *testing.T) {
	if VerifyREST(vectorPubKey, vectorSigPostHello, "hello", "1700000001", "POST") {
		t.Error("tampered timestamp should fail")
	}
}

func TestVerifyREST_GETIgnoresBody(t *testing.T) {
	// For GET, payload should be "" + ":" + timestamp regardless of "rawBody" arg.
	// We don't have a precomputed GET vector with the seed-0..31 keypair, but we
	// can verify the symmetric inverse: a POST vector should NOT verify when treated as GET.
	if VerifyREST(vectorPubKey, vectorSigPostHello, "hello", "1700000000", "GET") {
		t.Error("verifying POST signature against GET (which forces empty body) should fail")
	}
}

func TestVerifyREST_MalformedBase64(t *testing.T) {
	if VerifyREST("not!base64", vectorSigPostHello, "hello", "1700000000", "POST") {
		t.Error("malformed pubkey should fail")
	}
	if VerifyREST(vectorPubKey, "not!base64", "hello", "1700000000", "POST") {
		t.Error("malformed sig should fail")
	}
}

func TestVerifyTimestamp_KnownVector(t *testing.T) {
	if !VerifyTimestamp(vectorPubKey, vectorSigTsOnly, "1700000000") {
		t.Error("timestamp-only signature should verify")
	}
}

func TestVerifyTimestamp_Mismatch(t *testing.T) {
	if VerifyTimestamp(vectorPubKey, vectorSigTsOnly, "1700000001") {
		t.Error("timestamp mismatch should fail")
	}
}
