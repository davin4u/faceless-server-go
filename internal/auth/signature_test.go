package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/davin4u/faceless-server-go/internal/db"
)

type fakeDB struct {
	pubKey string
}

func (f *fakeDB) Get(ctx context.Context, q string, a ...any) (db.Row, error) {
	if a[0].(string) == f.pubKey {
		return db.Row{
			"id": "u1", "public_key": f.pubKey, "chat_public_key": "ck",
			"display_name": "Alice", "contact_code": "AAAA-2222",
		}, nil
	}
	return nil, nil
}
func (f *fakeDB) All(c context.Context, q string, a ...any) ([]db.Row, error) { return nil, nil }
func (f *fakeDB) Run(c context.Context, q string, a ...any) (db.Result, error) {
	return db.Result{}, nil
}
func (f *fakeDB) Exec(c context.Context, q string) error                 { return nil }
func (f *fakeDB) Tx(c context.Context, fn func(tx db.Tx) error) error    { return nil }
func (f *fakeDB) Close() error                                           { return nil }
func (f *fakeDB) InsertIgnore(_, _, _ string) string                     { return "" }
func (f *fakeDB) NowEpoch() string                                       { return "" }
func (f *fakeDB) Dialect() string                                        { return "sqlite" }

// signPOST returns (pubKeyB64, sigB64, exactBodyBytes, timestampStr).
func signPOST(t *testing.T, payload any) (string, string, []byte, string) {
	t.Helper()
	body, _ := json.Marshal(payload)
	return signWith(t, body, false)
}
func signGET(t *testing.T) (string, string, []byte, string) {
	t.Helper()
	return signWith(t, nil, true)
}
func signWith(t *testing.T, body []byte, isGet bool) (string, string, []byte, string) {
	t.Helper()
	pubB64, secB64 := freshKeypair(t)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	bodyStr := string(body)
	if isGet {
		bodyStr = ""
	}
	sig := signMessage(t, secB64, bodyStr+":"+ts)
	return pubB64, sig, body, ts
}

func TestRequireSignatureAuth_ValidPOST(t *testing.T) {
	pub, sig, body, ts := signPOST(t, map[string]any{"contactCode": "AAAA-2222"})
	d := &fakeDB{pubKey: pub}

	req := httptest.NewRequest("POST", "/api/contacts/add", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Public-Key", pub)
	req.Header.Set("X-Signature", sig)
	req.Header.Set("X-Timestamp", ts)

	rr := httptest.NewRecorder()
	called := false
	h := RequireSignatureAuth(d)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Body should still be readable downstream
		var got map[string]any
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("body unreadable downstream: %v", err)
		}
		if got["contactCode"] != "AAAA-2222" {
			t.Errorf("body = %+v", got)
		}
		u := UserFromCtx(r.Context())
		if u == nil || u.ID != "u1" {
			t.Errorf("user not in context: %+v", u)
		}
		called = true
		w.WriteHeader(204)
	}))
	h.ServeHTTP(rr, req)

	if !called {
		t.Fatalf("handler not called; status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestRequireSignatureAuth_RejectsBadSignature(t *testing.T) {
	pub, _, body, ts := signPOST(t, map[string]any{"x": 1})
	d := &fakeDB{pubKey: pub}
	req := httptest.NewRequest("POST", "/x", bytes.NewReader(body))
	req.Header.Set("X-Public-Key", pub)
	req.Header.Set("X-Signature", "AAAA")
	req.Header.Set("X-Timestamp", ts)
	rr := httptest.NewRecorder()
	RequireSignatureAuth(d)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("should not reach handler")
	})).ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestRequireSignatureAuth_RejectsExpiredTimestamp(t *testing.T) {
	pub, sig, body, _ := signPOST(t, map[string]any{"x": 1})
	old := strconv.FormatInt(time.Now().Unix()-10000, 10)
	d := &fakeDB{pubKey: pub}
	req := httptest.NewRequest("POST", "/x", bytes.NewReader(body))
	req.Header.Set("X-Public-Key", pub)
	req.Header.Set("X-Signature", sig)
	req.Header.Set("X-Timestamp", old)
	rr := httptest.NewRecorder()
	RequireSignatureAuth(d)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("should not reach handler")
	})).ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Errorf("status = %d", rr.Code)
	}
}

func TestRequireSignatureAuth_GETUsesEmptyBody(t *testing.T) {
	pub, sig, _, ts := signGET(t)
	d := &fakeDB{pubKey: pub}
	req := httptest.NewRequest("GET", "/api/contacts", nil)
	req.Header.Set("X-Public-Key", pub)
	req.Header.Set("X-Signature", sig)
	req.Header.Set("X-Timestamp", ts)

	rr := httptest.NewRecorder()
	called := false
	RequireSignatureAuth(d)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	})).ServeHTTP(rr, req)
	if !called {
		t.Errorf("GET should pass with empty-body signature; status=%d body=%s", rr.Code, rr.Body.String())
	}
}
