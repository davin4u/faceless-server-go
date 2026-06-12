package socketio

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/davin4u/faceless-server-go/internal/db"
	socketio "github.com/zishang520/socket.io/v2/socket"
)

type sockEntry struct {
	socket     *socketio.Socket
	socketType string
}

type Presence struct {
	io *socketio.Server
	d  db.DB

	mu          sync.RWMutex
	users       map[string]map[string]sockEntry // userID → socketID → entry
	timers      map[string]*time.Timer          // userID → pending offline timer
	debounce    time.Duration                   // 5s in prod
	broadcastFn func(userID string, online bool) // overridable for tests
}

// NewPresence constructs a live Presence wired to a real socket.io server.
func NewPresence(io *socketio.Server, d db.DB) *Presence {
	p := &Presence{
		io:       io,
		d:        d,
		users:    make(map[string]map[string]sockEntry),
		timers:   make(map[string]*time.Timer),
		debounce: 5 * time.Second,
	}
	p.broadcastFn = p.defaultBroadcast
	return p
}

// NewPresenceForTest constructs a Presence with no socket.io server attached;
// use AddRaw/RemoveRaw to manipulate state directly.
func NewPresenceForTest(d db.DB) *Presence {
	return &Presence{
		d:           d,
		users:       make(map[string]map[string]sockEntry),
		timers:      make(map[string]*time.Timer),
		debounce:    5 * time.Second,
		broadcastFn: func(string, bool) {},
	}
}

func (p *Presence) Add(userID string, s *socketio.Socket, socketType string) {
	p.AddRaw(userID, string(s.Id()), socketType)
	p.mu.Lock()
	if _, ok := p.users[userID]; ok {
		p.users[userID][string(s.Id())] = sockEntry{socket: s, socketType: socketType}
	}
	p.mu.Unlock()
}

func (p *Presence) Remove(userID string, s *socketio.Socket) {
	p.RemoveRaw(userID, string(s.Id()))
}

func (p *Presence) AddRaw(userID, socketID, socketType string) {
	p.mu.Lock()
	// Cancel any pending offline timer; if one was pending the user never fully
	// went offline — suppress the online broadcast to avoid a flickering presence.
	cancelledTimer := false
	if t, ok := p.timers[userID]; ok {
		t.Stop()
		delete(p.timers, userID)
		cancelledTimer = true
	}
	hadAppSocket := p.hasAppSocketLocked(userID)
	if _, ok := p.users[userID]; !ok {
		p.users[userID] = make(map[string]sockEntry)
	}
	p.users[userID][socketID] = sockEntry{socketType: socketType}
	// Only broadcast online=true when this is the first app socket AND there
	// was no pending offline timer (i.e. not a fast reconnect within debounce).
	shouldBroadcastOnline := socketType != "service" && !hadAppSocket && !cancelledTimer
	p.mu.Unlock()

	if shouldBroadcastOnline {
		p.broadcastFn(userID, true)
	}
}

func (p *Presence) RemoveRaw(userID, socketID string) {
	p.mu.Lock()
	sockets, ok := p.users[userID]
	if !ok {
		p.mu.Unlock()
		return
	}
	entry, ok := sockets[socketID]
	if !ok {
		p.mu.Unlock()
		return
	}
	delete(sockets, socketID)

	if len(sockets) == 0 {
		delete(p.users, userID)
		p.scheduleOfflineLocked(userID)
		p.mu.Unlock()
		return
	}
	if entry.socketType != "service" && !p.hasAppSocketLocked(userID) {
		p.scheduleOfflineLocked(userID)
		slog.Info("presence.lost_app_sockets", "user_id", userID,
			"remaining_service_sockets", len(sockets))
	}
	p.mu.Unlock()
}

func (p *Presence) scheduleOfflineLocked(userID string) {
	if t, ok := p.timers[userID]; ok {
		t.Stop()
	}
	debounce := p.debounce
	p.timers[userID] = time.AfterFunc(debounce, func() {
		p.mu.Lock()
		delete(p.timers, userID)
		hasApp := p.hasAppSocketLocked(userID)
		p.mu.Unlock()
		if !hasApp {
			p.broadcastFn(userID, false)
		}
	})
}

func (p *Presence) hasAppSocketLocked(userID string) bool {
	for _, e := range p.users[userID] {
		if e.socketType != "service" {
			return true
		}
	}
	return false
}

func (p *Presence) HasAppSocket(userID string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.hasAppSocketLocked(userID)
}

func (p *Presence) IsUserOnline(userID string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.users[userID]) > 0
}

func (p *Presence) GetConnectionCounts() ConnectionCounts {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var c ConnectionCounts
	for _, sockets := range p.users {
		for _, e := range sockets {
			if e.socketType == "service" {
				c.Service++
			} else {
				c.App++
			}
		}
	}
	return c
}

// EmitToUser sends event+payload to every socket of userID. No-op if offline.
func (p *Presence) EmitToUser(userID, event string, payload any) {
	p.mu.RLock()
	sockets := make([]*socketio.Socket, 0, len(p.users[userID]))
	for _, e := range p.users[userID] {
		if e.socket != nil {
			sockets = append(sockets, e.socket)
		}
	}
	p.mu.RUnlock()
	for _, s := range sockets {
		s.Emit(event, payload)
	}
}

// EmitToUserAppOnly sends event+payload to app sockets only (excludes service).
func (p *Presence) EmitToUserAppOnly(userID, event string, payload any) {
	p.mu.RLock()
	var sockets []*socketio.Socket
	for _, e := range p.users[userID] {
		if e.socketType != "service" && e.socket != nil {
			sockets = append(sockets, e.socket)
		}
	}
	p.mu.RUnlock()
	for _, s := range sockets {
		s.Emit(event, payload)
	}
}

func (p *Presence) defaultBroadcast(userID string, online bool) {
	contacts, err := p.acceptedContactIDs(context.Background(), userID)
	if err != nil {
		slog.Error("presence.lookup_contacts.error", "user_id", userID, "err", err)
		return
	}
	for _, cid := range contacts {
		p.EmitToUser(cid, "presence:update", map[string]any{
			"userId": userID, "online": online,
		})
	}
	slog.Info("presence.broadcast", "user_id", userID, "online", online, "recipients", len(contacts))
}

func (p *Presence) acceptedContactIDs(ctx context.Context, userID string) ([]string, error) {
	rows, err := p.d.All(ctx, `
		SELECT CASE WHEN user_id = ? THEN contact_id ELSE user_id END AS contact_id
		FROM contacts WHERE (user_id = ? OR contact_id = ?) AND status = 'accepted'`,
		userID, userID, userID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Str("contact_id"))
	}
	return out, nil
}

func (p *Presence) appSocketCount(userID string) int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	n := 0
	for _, e := range p.users[userID] {
		if e.socketType != "service" {
			n++
		}
	}
	return n
}

func (p *Presence) serviceSocketCount(userID string) int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	n := 0
	for _, e := range p.users[userID] {
		if e.socketType == "service" {
			n++
		}
	}
	return n
}

// WaitForAppSocket blocks until the user has an app socket or the timeout
// elapses. Returns true if an app socket appeared. Polling keeps it simple and
// the per-call counts are already mutex-guarded.
func (p *Presence) WaitForAppSocket(ctx context.Context, userID string, timeout time.Duration) bool {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(300 * time.Millisecond)
	defer tick.Stop()
	for {
		if p.HasAppSocket(userID) {
			return true
		}
		select {
		case <-deadline.C:
			return false
		case <-ctx.Done():
			return false
		case <-tick.C:
		}
	}
}
