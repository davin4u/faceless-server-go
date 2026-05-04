package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/davin4u/faceless-server-go/internal/auth"
	"github.com/davin4u/faceless-server-go/internal/db"
	"github.com/davin4u/faceless-server-go/internal/socketio"
)

type captureNotify struct {
	requests  []socketio.ContactRequestPayload
	accepted  []socketio.ContactAcceptedPayload
	requestTo []string
	acceptTo  []string
}

func (c *captureNotify) NotifyContactRequest(to string, p socketio.ContactRequestPayload) {
	c.requestTo = append(c.requestTo, to)
	c.requests = append(c.requests, p)
}
func (c *captureNotify) NotifyContactAccepted(to string, p socketio.ContactAcceptedPayload) {
	c.acceptTo = append(c.acceptTo, to)
	c.accepted = append(c.accepted, p)
}

func seedTwoUsers(t *testing.T, d db.DB) (a, b *auth.User) {
	t.Helper()
	ctx := context.Background()
	_, _ = d.Run(ctx, `INSERT INTO users (id, contact_code, display_name, public_key, chat_public_key) VALUES ('uA','AAAA-2222','A','pkA','ckA')`)
	_, _ = d.Run(ctx, `INSERT INTO users (id, contact_code, display_name, public_key, chat_public_key) VALUES ('uB','BBBB-3333','B','pkB','ckB')`)
	return &auth.User{ID: "uA", PublicKey: "pkA", ChatPublicKey: "ckA", DisplayName: "A", ContactCode: "AAAA-2222"},
		&auth.User{ID: "uB", PublicKey: "pkB", ChatPublicKey: "ckB", DisplayName: "B", ContactCode: "BBBB-3333"}
}

// callWithUser invokes h with a request whose context carries `u`.
func callWithUser(h http.Handler, method, path string, body []byte, u *auth.User) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req = req.WithContext(auth.WithUser(req.Context(), u))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestContacts_AddSendsRequestNotification(t *testing.T) {
	d := newSqlite(t)
	uA, uB := seedTwoUsers(t, d)
	notif := &captureNotify{}
	h := NewContacts(d, notif)

	body, _ := json.Marshal(map[string]string{"contactCode": uB.ContactCode})
	rr := callWithUser(h, "POST", "/add", body, uA)
	if rr.Code != 200 {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if len(notif.requestTo) != 1 || notif.requestTo[0] != "uB" {
		t.Errorf("request notify = %+v", notif.requestTo)
	}
	if notif.requests[0].DisplayName != "A" {
		t.Errorf("payload = %+v", notif.requests[0])
	}
}

func TestContacts_RejectsSelfAdd(t *testing.T) {
	d := newSqlite(t)
	uA, _ := seedTwoUsers(t, d)
	notif := &captureNotify{}
	h := NewContacts(d, notif)
	body, _ := json.Marshal(map[string]string{"contactCode": uA.ContactCode})
	rr := callWithUser(h, "POST", "/add", body, uA)
	if rr.Code != 400 {
		t.Errorf("status = %d", rr.Code)
	}
}

func TestContacts_AcceptCreatesReverseRowAndNotifies(t *testing.T) {
	d := newSqlite(t)
	_, uB := seedTwoUsers(t, d)
	ctx := context.Background()
	_, _ = d.Run(ctx, `INSERT INTO contacts (user_id, contact_id, status) VALUES ('uA','uB','pending')`)

	notif := &captureNotify{}
	h := NewContacts(d, notif)
	body, _ := json.Marshal(map[string]string{"userId": "uA"})
	rr := callWithUser(h, "POST", "/accept", body, uB)
	if rr.Code != 200 {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	rows, _ := d.All(ctx, `SELECT user_id, contact_id, status FROM contacts ORDER BY user_id`)
	if len(rows) != 2 {
		t.Errorf("rows = %+v", rows)
	}
	if len(notif.acceptTo) != 1 || notif.acceptTo[0] != "uA" {
		t.Errorf("accept notify = %+v", notif.acceptTo)
	}
	if notif.accepted[0].ChatPublicKey != "ckB" {
		t.Errorf("payload = %+v", notif.accepted[0])
	}
}

func TestContacts_GetListReturnsAccepted(t *testing.T) {
	d := newSqlite(t)
	uA, _ := seedTwoUsers(t, d)
	ctx := context.Background()
	_, _ = d.Run(ctx, `INSERT INTO contacts (user_id, contact_id, status) VALUES ('uA','uB','accepted')`)
	_, _ = d.Run(ctx, `INSERT INTO contacts (user_id, contact_id, status) VALUES ('uB','uA','accepted')`)

	notif := &captureNotify{}
	h := NewContacts(d, notif)
	rr := callWithUser(h, "GET", "/", nil, uA)
	if rr.Code != 200 {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	contacts := resp["contacts"].([]any)
	if len(contacts) != 1 {
		t.Errorf("contacts = %+v", contacts)
	}
	c0 := contacts[0].(map[string]any)
	if c0["displayName"] != "B" {
		t.Errorf("c0 = %+v", c0)
	}
}

func TestContacts_RegenerateRetiresOldCode(t *testing.T) {
	d := newSqlite(t)
	uA, _ := seedTwoUsers(t, d)
	notif := &captureNotify{}
	h := NewContacts(d, notif)
	rr := callWithUser(h, "POST", "/regenerate-code", []byte(`{}`), uA)
	if rr.Code != 200 {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	row, _ := d.Get(context.Background(), `SELECT 1 FROM retired_codes WHERE code = ?`, "AAAA-2222")
	if row == nil {
		t.Error("old code should be in retired_codes")
	}
}
