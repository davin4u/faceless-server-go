package routes

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/davin4u/faceless-server-go/internal/socketio"
)

func TestAdminStats_HappyPath(t *testing.T) {
	d := newSqlite(t)
	// Seed one user and one undelivered message
	_, _ = d.Run(context.Background(), `INSERT INTO users (id, contact_code, display_name, public_key) VALUES ('u','AAAA-2222','A','pk')`)
	_, _ = d.Run(context.Background(), `INSERT INTO messages (id, sender_id, receiver_id, ciphertext, nonce, timestamp, delivered) VALUES ('m','u','u','c','n',1700000000,0)`)
	_, _ = d.Run(context.Background(), `INSERT INTO daily_stats (date, messages_sent) VALUES ('2026-05-01', 5)`)

	h := NewAdmin(d, "topsecret", socketio.NoopCounter{})
	req := httptest.NewRequest("GET", "/api/admin/stats", nil)
	req.Header.Set("Authorization", "Bearer topsecret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	users := resp["users"].(map[string]any)
	if int(users["total"].(float64)) != 1 {
		t.Errorf("users.total = %v", users["total"])
	}
	msg := resp["messages"].(map[string]any)
	if int(msg["undelivered"].(float64)) != 1 {
		t.Errorf("undelivered = %v", msg["undelivered"])
	}
	conns := resp["connections"].(map[string]any)
	if int(conns["app"].(float64)) != 0 || int(conns["service"].(float64)) != 0 {
		t.Errorf("connections = %+v", conns)
	}
	system := resp["system"].(map[string]any)
	if system["uptimeSeconds"] == nil || system["nodeVersion"] == nil {
		t.Errorf("system missing keys: %+v", system)
	}
	dailyStats := resp["dailyStats"].([]any)
	if len(dailyStats) != 1 {
		t.Errorf("dailyStats = %+v", dailyStats)
	}
}

func TestAdminStats_RejectsMissingBearer(t *testing.T) {
	d := newSqlite(t)
	h := NewAdmin(d, "topsecret", socketio.NoopCounter{})
	req := httptest.NewRequest("GET", "/api/admin/stats", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Errorf("status = %d", rr.Code)
	}
}

func TestHealth_OK(t *testing.T) {
	rr := httptest.NewRecorder()
	NewHealth().ServeHTTP(rr, httptest.NewRequest("GET", "/health", nil))
	if rr.Code != 200 {
		t.Fatalf("status = %d", rr.Code)
	}
	var resp map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["status"] != "ok" {
		t.Errorf("body = %+v", resp)
	}
}
