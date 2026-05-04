package socketio

import (
	"github.com/davin4u/faceless-server-go/internal/db"
	socketio "github.com/zishang520/socket.io/v2/socket"
)

// Presence tracks online users and their socket connections.
// This is a stub — the real implementation is added in Task 23.
type Presence struct {
	io *socketio.Server
	d  db.DB
}

// NewPresence creates a new Presence tracker stub.
func NewPresence(io *socketio.Server, d db.DB) *Presence {
	return &Presence{io: io, d: d}
}

// Add registers a socket connection for a user.
func (p *Presence) Add(_ string, _ *socketio.Socket, _ string) {}

// Remove unregisters a socket connection for a user.
func (p *Presence) Remove(_ string, _ *socketio.Socket) {}

// EmitToUser sends an event to all active sockets of a user.
func (p *Presence) EmitToUser(_ string, _ string, _ any) {}

// GetConnectionCounts returns current app and service socket counts.
func (p *Presence) GetConnectionCounts() ConnectionCounts { return ConnectionCounts{} }
