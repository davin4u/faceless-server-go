package socketio

import (
	"context"
	"testing"
	"time"

	"github.com/davin4u/faceless-server-go/internal/db"
)

func seedCallUsers(t *testing.T, d db.DB) {
	t.Helper()
	ctx := context.Background()
	for _, u := range []string{"caller-id", "callee-id"} {
		if _, err := d.Run(ctx,
			`INSERT INTO users (id, contact_code, display_name, public_key) VALUES (?,?,?,?)`,
			u, u+"-code", u+"-name", "pk_"+u); err != nil {
			t.Fatalf("seed %s: %v", u, err)
		}
	}
}

func missedCount(t *testing.T, d db.DB, callee string) int {
	t.Helper()
	rows, err := d.All(context.Background(),
		`SELECT id FROM pending_events WHERE user_id=? AND event_type='call:missed'`, callee)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	return len(rows)
}

func newCallServer(d db.DB) *Server { return &Server{d: d, presence: NewPresenceForTest(d)} }

func setTiming(k, caller, callee, callType string, answered bool) {
	callMu.Lock()
	tm := &callTiming{OfferTime: time.Now(), CallerID: caller, CalleeID: callee, CallType: callType}
	if answered {
		tm.AnswerTime = time.Now()
		activeCalls[k] = time.Now()
	}
	callTimings[k] = tm
	callIceCount[k] = &iceCounts{}
	callMu.Unlock()
}

func TestEndCall_UnansweredOfflineCallee_EnqueuesMissed(t *testing.T) {
	d := newSqlite(t)
	seedCallUsers(t, d)
	s := newCallServer(d)
	k := callKey("caller-id", "callee-id")
	setTiming(k, "caller-id", "callee-id", "video", false) // not answered
	s.endCall(k, "hangup", "caller-id")                    // callee has no app socket
	if got := missedCount(t, d, "callee-id"); got != 1 {
		t.Fatalf("want 1 missed event, got %d", got)
	}
}

func TestEndCall_Answered_NoMissed(t *testing.T) {
	d := newSqlite(t)
	seedCallUsers(t, d)
	s := newCallServer(d)
	k := callKey("caller-id", "callee-id")
	setTiming(k, "caller-id", "callee-id", "voice", true) // answered
	s.endCall(k, "hangup", "caller-id")
	if got := missedCount(t, d, "callee-id"); got != 0 {
		t.Fatalf("answered call must not enqueue missed, got %d", got)
	}
}

func TestEndCall_CalleeHasAppSocket_NoMissed(t *testing.T) {
	d := newSqlite(t)
	seedCallUsers(t, d)
	s := newCallServer(d)
	s.presence.AddRaw("callee-id", "s1", "app") // callee can self-record live
	k := callKey("caller-id", "callee-id")
	setTiming(k, "caller-id", "callee-id", "voice", false)
	s.endCall(k, "hangup", "caller-id")
	if got := missedCount(t, d, "callee-id"); got != 0 {
		t.Fatalf("callee with app socket self-records; server must not enqueue, got %d", got)
	}
}

func TestEndCall_CalleeRejected_NoMissed(t *testing.T) {
	d := newSqlite(t)
	seedCallUsers(t, d)
	s := newCallServer(d)
	k := callKey("caller-id", "callee-id")
	setTiming(k, "caller-id", "callee-id", "voice", false)
	s.endCall(k, "reject", "callee-id") // active decline, not a miss
	if got := missedCount(t, d, "callee-id"); got != 0 {
		t.Fatalf("active decline is not a missed call, got %d", got)
	}
}
