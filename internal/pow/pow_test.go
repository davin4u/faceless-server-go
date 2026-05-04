package pow

import (
	"crypto/sha256"
	"strings"
	"testing"
)

func solve(challenge string, difficulty int) int {
	for n := 0; ; n++ {
		h := sha256.Sum256([]byte(challenge + ":" + itoa(n)))
		if hasLeadingZeroBits(h[:], difficulty) {
			return n
		}
	}
}

// Local helper to avoid pulling in strconv just for tests.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func TestGenerateAndVerify(t *testing.T) {
	s := New(8) // difficulty 8 is fast to solve
	c := s.Generate("register")

	if !strings.HasPrefix(c.Challenge, "register:") {
		t.Errorf("challenge prefix: %q", c.Challenge)
	}
	if c.Difficulty != 8 {
		t.Errorf("difficulty = %d", c.Difficulty)
	}

	nonce := solve(c.Challenge, c.Difficulty)
	if !s.Verify(c.Challenge, nonce) {
		t.Error("solve+verify should succeed")
	}
}

func TestVerify_SingleUse(t *testing.T) {
	s := New(8)
	c := s.Generate("register")
	nonce := solve(c.Challenge, c.Difficulty)
	if !s.Verify(c.Challenge, nonce) {
		t.Fatal("first verify should pass")
	}
	if s.Verify(c.Challenge, nonce) {
		t.Error("second verify should fail (single-use)")
	}
}

func TestVerify_UnknownChallenge(t *testing.T) {
	s := New(8)
	if s.Verify("unknown:0:00", 0) {
		t.Error("unknown challenge should not verify")
	}
}

func TestHasLeadingZeroBits(t *testing.T) {
	// 0x00 0x80 = 8 leading zero bits then a 1 bit
	if !hasLeadingZeroBits([]byte{0x00, 0x80}, 8) {
		t.Error("8 zeros should pass at difficulty 8")
	}
	if hasLeadingZeroBits([]byte{0x00, 0x80}, 9) {
		t.Error("0x00 0x80 should fail at difficulty 9 (the 9th bit is 1)")
	}
	if !hasLeadingZeroBits([]byte{0x00, 0x00, 0x80}, 16) {
		t.Error("16 zeros should pass at difficulty 16")
	}
	if !hasLeadingZeroBits([]byte{0x0F}, 4) {
		t.Error("0x0F = 0000_1111 should pass at difficulty 4")
	}
	if hasLeadingZeroBits([]byte{0x0F}, 5) {
		t.Error("0x0F should fail at difficulty 5")
	}
}

func TestVerify_Expiry(t *testing.T) {
	s := New(8)
	c := s.Generate("register")
	// Force expiry
	s.mu.Lock()
	if e, ok := s.store[c.Challenge]; ok {
		e.ExpiresUnixMs = 1
		s.store[c.Challenge] = e
	}
	s.mu.Unlock()
	nonce := solve(c.Challenge, c.Difficulty)
	if s.Verify(c.Challenge, nonce) {
		t.Error("expired challenge should not verify")
	}
}
