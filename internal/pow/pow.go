// Package pow implements the SHA-256 partial-collision proof-of-work used to
// gate registration. Mirrors /server/src/utils/pow.js bit-for-bit.
package pow

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"sync"
	"time"
)

type Challenge struct {
	Challenge  string `json:"challenge"`
	Difficulty int    `json:"difficulty"`
}

type entry struct {
	Difficulty    int
	ExpiresUnixMs int64
	Used          bool
}

type Service struct {
	difficulty int
	mu         sync.Mutex
	store      map[string]entry
}

// New returns a Service with the given default difficulty (leading zero bits).
func New(difficulty int) *Service {
	return &Service{
		difficulty: difficulty,
		store:      make(map[string]entry),
	}
}

func (s *Service) Generate(action string) Challenge {
	var b [8]byte
	_, _ = rand.Read(b[:])
	c := fmt.Sprintf("%s:%d:%s", action, time.Now().Unix(), hex.EncodeToString(b[:]))

	s.mu.Lock()
	s.store[c] = entry{
		Difficulty:    s.difficulty,
		ExpiresUnixMs: time.Now().Add(5 * time.Minute).UnixMilli(),
		Used:          false,
	}
	s.mu.Unlock()

	return Challenge{Challenge: c, Difficulty: s.difficulty}
}

func (s *Service) Verify(challenge string, nonce int) bool {
	s.mu.Lock()
	e, ok := s.store[challenge]
	if !ok {
		s.mu.Unlock()
		return false
	}
	now := time.Now().UnixMilli()
	if e.Used {
		s.mu.Unlock()
		return false
	}
	if e.ExpiresUnixMs < now {
		delete(s.store, challenge)
		s.mu.Unlock()
		return false
	}
	e.Used = true
	s.store[challenge] = e
	difficulty := e.Difficulty
	s.mu.Unlock()

	hash := sha256.Sum256([]byte(challenge + ":" + strconv.Itoa(nonce)))
	return hasLeadingZeroBits(hash[:], difficulty)
}

// StartGC purges expired entries every 60 seconds until ctx is done. The Node
// implementation runs an interval; we match that.
func (s *Service) StartGC(stop <-chan struct{}) {
	t := time.NewTicker(60 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			s.gc()
		}
	}
}

func (s *Service) gc() {
	now := time.Now().UnixMilli()
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, e := range s.store {
		if e.ExpiresUnixMs < now {
			delete(s.store, k)
		}
	}
}

// hasLeadingZeroBits returns true if the first `difficulty` bits of hash are zero.
// Matches the JS port byte-for-byte.
func hasLeadingZeroBits(hash []byte, difficulty int) bool {
	remaining := difficulty
	for i := 0; i < len(hash) && remaining > 0; i++ {
		if remaining >= 8 {
			if hash[i] != 0 {
				return false
			}
			remaining -= 8
		} else {
			mask := byte(0xff << (8 - remaining))
			if hash[i]&mask != 0 {
				return false
			}
			remaining = 0
		}
	}
	return true
}
