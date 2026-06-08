package routes

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
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

	// Seed an inviter so registration can consume the invite.
	if _, err := d.Run(context.Background(),
		`INSERT INTO users (id, contact_code, display_name, public_key, invitation_code, invitation_code_usages)
		 VALUES ('inv0','AAAA-1111','Inviter Zero','pkinv0','HAPPY-INV',1)`); err != nil {
		t.Fatalf("seed inviter: %v", err)
	}

	rr := registerWithInvite(t, mux, "pkA", "ckA", "Tester One", "HAPPY-INV")

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
	if ic, ok := resp["invitationCode"].(string); !ok || len(ic) != 9 {
		t.Errorf("invitationCode = %v", resp["invitationCode"])
	}
	if u, ok := resp["invitationCodeUsages"].(float64); !ok || int(u) != 3 {
		t.Errorf("invitationCodeUsages = %v", resp["invitationCodeUsages"])
	}
}

func TestRegister_RejectsInvalidPow(t *testing.T) {
	d := newSqlite(t)
	mux := NewAuth(d, pow.New(20))
	// Seed an inviter so the invite check passes; PoW should still reject.
	if _, err := d.Run(context.Background(),
		`INSERT INTO users (id, contact_code, display_name, public_key, invitation_code, invitation_code_usages)
		 VALUES ('inv1','BBBB-2222','Inviter One','pkinv1','BAD-POW0',5)`); err != nil {
		t.Fatalf("seed inviter: %v", err)
	}
	body, _ := json.Marshal(map[string]any{
		"challenge": "garbage", "nonce": 0,
		"publicKey": "p", "chatPublicKey": "c", "displayName": "X",
		"invitationCode": "BAD-POW0",
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

// solvePoW obtains a challenge from the mux and returns (challenge, nonce).
func solvePoW(t *testing.T, mux http.Handler) (string, int) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"action": "register"})
	req := httptest.NewRequest("POST", "/api/pow/challenge", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	var ch map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &ch)
	chStr := ch["challenge"].(string)
	diff := int(ch["difficulty"].(float64))
	nonce := 0
	for {
		h := sha256.Sum256([]byte(chStr + ":" + strconv.Itoa(nonce)))
		if hasZeros(h[:], diff) {
			break
		}
		nonce++
	}
	return chStr, nonce
}

// registerWithInvite performs a full register call with the given invitationCode and returns the recorder.
func registerWithInvite(t *testing.T, mux http.Handler, publicKey, chatPublicKey, displayName, invitationCode string) *httptest.ResponseRecorder {
	t.Helper()
	chStr, nonce := solvePoW(t, mux)
	body, _ := json.Marshal(map[string]any{
		"challenge":      chStr,
		"nonce":          nonce,
		"publicKey":      publicKey,
		"chatPublicKey":  chatPublicKey,
		"displayName":    displayName,
		"invitationCode": invitationCode,
	})
	req := httptest.NewRequest("POST", "/api/register", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return rr
}

// dbGetInt runs a query and returns the first column of the first row as int64.
func dbGetInt(t *testing.T, d db.DB, query string, args ...any) int64 {
	t.Helper()
	row, err := d.Get(context.Background(), query, args...)
	if err != nil {
		t.Fatalf("dbGetInt query=%q: %v", query, err)
	}
	if row == nil {
		t.Fatalf("dbGetInt query=%q: no row returned", query)
	}
	for _, v := range row {
		switch n := v.(type) {
		case int64:
			return n
		case int32:
			return int64(n)
		case int:
			return int64(n)
		case float64:
			return int64(n)
		}
	}
	return 0
}

func TestRegisterConsumesInvite(t *testing.T) {
	d := newSqlite(t)
	p := pow.New(8)
	mux := NewAuth(d, p)

	// Seed an inviter with a known invitation code and 1 usage left.
	ctx := context.Background()
	if _, err := d.Run(ctx,
		`INSERT INTO users (id, contact_code, display_name, public_key, invitation_code, invitation_code_usages)
		 VALUES ('inviter','CCCC-4444','Inviter','pkinv','INVT-CODE',1)`); err != nil {
		t.Fatalf("seed inviter: %v", err)
	}

	// First registration with the invite succeeds and consumes the use.
	rr := registerWithInvite(t, mux, "pkB", "ckB", "User Beta", "INVT-CODE")
	if rr.Code != 201 {
		t.Fatalf("want 201, got %d body=%s", rr.Code, rr.Body.String())
	}

	// Inviter's usages should now be 0.
	usages := dbGetInt(t, d, `SELECT invitation_code_usages FROM users WHERE id='inviter'`)
	if usages != 0 {
		t.Fatalf("want 0 usages after consume, got %d", usages)
	}

	// New user got their own invitation code + 3 usages.
	newUsages := dbGetInt(t, d,
		`SELECT invitation_code_usages FROM users WHERE invitation_code IS NOT NULL AND id != 'inviter'`)
	if newUsages != 3 {
		t.Fatalf("new user should start with 3 usages, got %d", newUsages)
	}

	// Second registration with the now-exhausted code is rejected.
	rr2 := registerWithInvite(t, mux, "pkC", "ckC", "User Gamma", "INVT-CODE")
	if rr2.Code != 400 {
		t.Fatalf("want 400 for exhausted invite, got %d body=%s", rr2.Code, rr2.Body.String())
	}
}

func TestRegisterRejectsMissingInvite(t *testing.T) {
	d := newSqlite(t)
	p := pow.New(8)
	mux := NewAuth(d, p)

	rr := registerWithInvite(t, mux, "pkD", "ckD", "User Delta", "")
	if rr.Code != 400 {
		t.Fatalf("want 400 for missing invite, got %d body=%s", rr.Code, rr.Body.String())
	}
}

// postResult wraps an httptest.ResponseRecorder for inline assertions.
type postResult struct {
	*httptest.ResponseRecorder
}

// postJSON sends a POST to the mux at path with the given raw JSON body.
func postJSON(t *testing.T, mux http.Handler, path, body string) postResult {
	t.Helper()
	req := httptest.NewRequest("POST", path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return postResult{rr}
}

// valid parses {"valid":bool} from the response body and returns the bool.
func (r postResult) valid(t *testing.T) bool {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(r.Body.Bytes(), &resp); err != nil {
		t.Fatalf("postResult.valid: json unmarshal error: %v (body: %s)", err, r.Body.String())
	}
	v, ok := resp["valid"].(bool)
	if !ok {
		t.Fatalf("postResult.valid: 'valid' field missing or not bool in: %s", r.Body.String())
	}
	return v
}

func TestInviteValidate(t *testing.T) {
	d := newSqlite(t)
	mux := NewAuth(d, pow.New(8))
	ctx := context.Background()

	// Seed: one user with a valid code and usages >= 1
	if _, err := d.Run(ctx,
		`INSERT INTO users (id, contact_code, display_name, public_key, invitation_code, invitation_code_usages)
		 VALUES ('inv','DDDD-5555','Inv','pkv','GOOD-CODE',2)`); err != nil {
		t.Fatalf("seed inv: %v", err)
	}
	// Seed: one user with usages = 0 (exhausted)
	if _, err := d.Run(ctx,
		`INSERT INTO users (id, contact_code, display_name, public_key, invitation_code, invitation_code_usages)
		 VALUES ('inv0','EEEE-6666','Inv0','pkv0','SPENT-COD',0)`); err != nil {
		t.Fatalf("seed inv0: %v", err)
	}

	if got := postJSON(t, mux, "/api/invite/validate", `{"invitationCode":"GOOD-CODE"}`).valid(t); !got {
		t.Fatalf("GOOD-CODE should be valid")
	}
	if got := postJSON(t, mux, "/api/invite/validate", `{"invitationCode":"SPENT-COD"}`).valid(t); got {
		t.Fatalf("SPENT-COD should be invalid (0 usages)")
	}
	if got := postJSON(t, mux, "/api/invite/validate", `{"invitationCode":"NOPE-NOPE"}`).valid(t); got {
		t.Fatalf("unknown code should be invalid")
	}
	// Empty code → valid:false
	if got := postJSON(t, mux, "/api/invite/validate", `{"invitationCode":""}`).valid(t); got {
		t.Fatalf("empty code should be invalid")
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
