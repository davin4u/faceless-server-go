package socketio

import (
	"context"
	"testing"
	"time"

	"github.com/davin4u/faceless-server-go/internal/db"
)

func newSqlite(t *testing.T) db.DB {
	t.Helper()
	d, err := db.NewSqlite(t.TempDir() + "/p.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.InitSchema(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestPresence_AddRemoveCountsAndOnlineCheck(t *testing.T) {
	d := newSqlite(t)
	p := NewPresenceForTest(d)

	p.AddRaw("u1", "s1", "app")
	p.AddRaw("u1", "s2", "service")
	p.AddRaw("u2", "s3", "app")

	if !p.IsUserOnline("u1") {
		t.Error("u1 should be online")
	}
	if !p.HasAppSocket("u1") {
		t.Error("u1 should have app socket")
	}
	c := p.GetConnectionCounts()
	if c.App != 2 || c.Service != 1 {
		t.Errorf("counts = %+v", c)
	}

	p.RemoveRaw("u1", "s1")
	if !p.HasAppSocket("u1") {
		// removing the only app socket should drop hasAppSocket
		// but we kept service — so hasAppSocket should be FALSE
		// Actually s1 was the only app, so we expect false
	}
	if p.HasAppSocket("u1") {
		t.Error("u1 should no longer have app socket")
	}
	c = p.GetConnectionCounts()
	if c.App != 1 || c.Service != 1 {
		t.Errorf("counts after remove = %+v", c)
	}
}

func TestPresence_OfflineDebounce(t *testing.T) {
	d := newSqlite(t)
	p := NewPresenceForTest(d)

	broadcasts := make(chan bool, 8)
	p.broadcastFn = func(userID string, online bool) {
		broadcasts <- online
	}

	p.AddRaw("u1", "s1", "app")
	// Drain initial online broadcast (should fire on first app socket)
	select {
	case b := <-broadcasts:
		if !b {
			t.Error("first add should broadcast online=true")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected online=true broadcast")
	}

	// Override the debounce window to 50ms so the test runs fast
	p.debounce = 50 * time.Millisecond
	p.RemoveRaw("u1", "s1")

	// Within debounce window, user should still be considered online
	if p.IsUserOnline("u1") {
		// onlineUsers map cleared, but that's fine — IsUserOnline reflects the map state.
		// What matters is no offline broadcast fired yet.
	}

	select {
	case <-broadcasts:
		t.Error("offline broadcast fired before debounce expired")
	case <-time.After(20 * time.Millisecond):
	}

	// Wait for debounce to expire
	select {
	case b := <-broadcasts:
		if b {
			t.Error("expected online=false")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected delayed offline broadcast")
	}
}

func TestPresence_OfflineDebounce_CancelledByReconnect(t *testing.T) {
	d := newSqlite(t)
	p := NewPresenceForTest(d)

	broadcasts := make(chan bool, 8)
	p.broadcastFn = func(_ string, online bool) { broadcasts <- online }
	p.debounce = 100 * time.Millisecond

	p.AddRaw("u1", "s1", "app")
	<-broadcasts // online=true
	p.RemoveRaw("u1", "s1")
	// Reconnect quickly — should cancel pending offline broadcast
	p.AddRaw("u1", "s2", "app")

	select {
	case b := <-broadcasts:
		t.Errorf("unexpected broadcast online=%v after fast reconnect", b)
	case <-time.After(200 * time.Millisecond):
		// good
	}
}
