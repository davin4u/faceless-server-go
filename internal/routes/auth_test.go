package routes

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/davin4u/faceless-server-go/internal/db"
	"github.com/davin4u/faceless-server-go/internal/pow"
)

func newSqlite(t *testing.T) db.DB {
	t.Helper()
	d, err := db.NewSqlite(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.InitSchema(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestPowChallenge_ReturnsChallengeAndDifficulty(t *testing.T) {
	d := newSqlite(t)
	p := pow.New(8)
	mux := NewAuth(d, p)

	body, _ := json.Marshal(map[string]string{"action": "register"})
	req := httptest.NewRequest("POST", "/api/pow/challenge", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if c, _ := resp["challenge"].(string); c == "" {
		t.Errorf("missing challenge: %+v", resp)
	}
	if int(resp["difficulty"].(float64)) != 8 {
		t.Errorf("difficulty = %v", resp["difficulty"])
	}
}

func TestRegister_HappyPath(t *testing.T) {
	d := newSqlite(t)
	p := pow.New(8)
	mux := NewAuth(d, p)

	// 1. Get challenge
	body, _ := json.Marshal(map[string]string{"action": "register"})
	req := httptest.NewRequest("POST", "/api/pow/challenge", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	var ch map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &ch)
	chStr := ch["challenge"].(string)
	diff := int(ch["difficulty"].(float64))

	// 2. Solve
	nonce := 0
	for {
		h := sha256.Sum256([]byte(chStr + ":" + strconv.Itoa(nonce)))
		if hasZeros(h[:], diff) {
			break
		}
		nonce++
	}

	// 3. Register
	regBody, _ := json.Marshal(map[string]any{
		"challenge":     chStr,
		"nonce":         nonce,
		"publicKey":     "pkA",
		"chatPublicKey": "ckA",
		"displayName":   "Tester One",
	})
	req = httptest.NewRequest("POST", "/api/register", bytes.NewReader(regBody))
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != 201 {
		t.Fatalf("register status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["displayName"] != "Tester One" {
		t.Errorf("displayName = %v", resp["displayName"])
	}
	if cc, ok := resp["contactCode"].(string); !ok || len(cc) != 9 {
		t.Errorf("contactCode = %v", resp["contactCode"])
	}
}

func TestRegister_RejectsInvalidPow(t *testing.T) {
	d := newSqlite(t)
	mux := NewAuth(d, pow.New(20))
	body, _ := json.Marshal(map[string]any{
		"challenge": "garbage", "nonce": 0,
		"publicKey": "p", "chatPublicKey": "c", "displayName": "X",
	})
	req := httptest.NewRequest("POST", "/api/register", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != 400 {
		t.Errorf("status = %d", rr.Code)
	}
}

func TestRecover_NotFound(t *testing.T) {
	d := newSqlite(t)
	mux := NewAuth(d, pow.New(8))
	body, _ := json.Marshal(map[string]string{"publicKey": "missing"})
	req := httptest.NewRequest("POST", "/api/recover", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != 404 {
		t.Errorf("status = %d", rr.Code)
	}
}

func TestGenerateName_ReturnsName(t *testing.T) {
	d := newSqlite(t)
	mux := NewAuth(d, pow.New(8))
	req := httptest.NewRequest("POST", "/api/generate-name", bytes.NewReader([]byte(`{}`)))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["name"] == "" {
		t.Error("empty name")
	}
}

// Local copy of hasLeadingZeroBits for tests
func hasZeros(h []byte, d int) bool {
	rem := d
	for i := 0; i < len(h) && rem > 0; i++ {
		if rem >= 8 {
			if h[i] != 0 {
				return false
			}
			rem -= 8
		} else {
			m := byte(0xff << (8 - rem))
			if h[i]&m != 0 {
				return false
			}
			rem = 0
		}
	}
	return true
}
