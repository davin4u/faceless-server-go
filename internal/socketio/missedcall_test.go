package socketio

import (
	"context"
	"strings"
	"testing"
)

func TestEnqueueMissedCall(t *testing.T) {
	ctx := context.Background()
	d := newSqlite(t) // reuse the helper defined in presence_test.go

	// Seed the two users so the pending_events FK constraint is satisfied.
	for _, u := range []struct{ id, pk string }{
		{"caller-id", "pk_caller"},
		{"callee-id", "pk_callee"},
	} {
		if _, err := d.Run(ctx,
			`INSERT INTO users (id, contact_code, display_name, public_key) VALUES (?, ?, ?, ?)`,
			u.id, u.id+"-code", u.id+"-name", u.pk,
		); err != nil {
			t.Fatalf("seed user %s: %v", u.id, err)
		}
	}

	if err := enqueueMissedCall(ctx, d, "caller-id", "callee-id", "video"); err != nil {
		t.Fatalf("enqueueMissedCall: %v", err)
	}
	row, err := d.Get(ctx, `SELECT event_type, payload FROM pending_events WHERE user_id='callee-id'`)
	if err != nil || row == nil {
		t.Fatalf("expected a pending event, err=%v row=%v", err, row)
	}
	if row.Str("event_type") != "call:missed" {
		t.Fatalf("want call:missed, got %q", row.Str("event_type"))
	}
	payload := row.Str("payload")
	if !strings.Contains(payload, `"from":"caller-id"`) {
		t.Fatalf("payload missing from field: %s", payload)
	}
	if !strings.Contains(payload, `"callType":"video"`) {
		t.Fatalf("payload missing callType field: %s", payload)
	}
}
