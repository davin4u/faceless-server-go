// Package socketio exposes a Notifier interface so non-socket packages can
// push events to online users without importing the full socket.io package.
//
// The concrete server implementation in server.go implements this interface.
package socketio

// Notifier emits server→client Socket.IO events to any online sockets of userID.
// No-op (or return false) if the user is offline.
type Notifier interface {
	NotifyContactRequest(toUserID string, payload ContactRequestPayload)
	NotifyContactAccepted(toUserID string, payload ContactAcceptedPayload)
}

type ContactRequestPayload struct {
	ID          string `json:"id"`
	ContactCode string `json:"contactCode"`
	DisplayName string `json:"displayName"`
}

type ContactAcceptedPayload struct {
	ID            string `json:"id"`
	ContactCode   string `json:"contactCode"`
	DisplayName   string `json:"displayName"`
	PublicKey     string `json:"publicKey"`
	ChatPublicKey string `json:"chatPublicKey"`
}

// NoopNotifier is a placeholder used until the real socket.io server is wired.
type NoopNotifier struct{}

func (NoopNotifier) NotifyContactRequest(string, ContactRequestPayload)   {}
func (NoopNotifier) NotifyContactAccepted(string, ContactAcceptedPayload) {}

// ConnectionCounts is consumed by the admin /stats handler.
type ConnectionCounts struct {
	App     int `json:"app"`
	Service int `json:"service"`
}

type ConnectionCounter interface {
	GetConnectionCounts() ConnectionCounts
}

// NoopCounter returns zero counts; used in tests.
type NoopCounter struct{}

func (NoopCounter) GetConnectionCounts() ConnectionCounts { return ConnectionCounts{} }
