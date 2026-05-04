package socketio

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/davin4u/faceless-server-go/internal/crypto"
	"github.com/davin4u/faceless-server-go/internal/db"
	"github.com/google/uuid"
	"github.com/zishang520/engine.io/v2/types"
	socketio "github.com/zishang520/socket.io/v2/socket"
)

// Server wraps zishang520 Socket.IO with our auth, presence, chat, signaling,
// and delivery handlers. It also implements Notifier and ConnectionCounter so
// the REST routes can push events without importing the library directly.
type Server struct {
	io       *socketio.Server
	d        db.DB
	logICE   bool
	presence *Presence
	mu       sync.RWMutex
}

// New constructs the server but does not yet attach it to an HTTP mux.
func New(d db.DB, logICE bool) *Server {
	cfg := socketio.DefaultServerOptions()
	cfg.SetCors(&types.Cors{Origin: "*", Methods: []string{"GET", "POST"}})
	io := socketio.NewServer(nil, cfg)
	s := &Server{
		io:       io,
		d:        d,
		logICE:   logICE,
		presence: NewPresence(io, d),
	}
	io.Use(s.authMiddleware)
	io.On("connection", s.onConnect) //nolint:errcheck
	return s
}

// Handler returns the http.Handler that serves the Engine.IO/Socket.IO
// transport. Mount it on `/socket.io/`.
func (s *Server) Handler() http.Handler {
	return s.io.ServeHandler(nil)
}

// Close shuts down the server gracefully.
func (s *Server) Close(_ context.Context) error {
	var closeErr error
	done := make(chan struct{})
	s.io.Close(func(err error) {
		closeErr = err
		close(done)
	})
	select {
	case <-done:
		return closeErr
	case <-time.After(5 * time.Second):
		return nil
	}
}

// authMiddleware verifies handshake.auth.{publicKey,timestamp,signature} and
// looks up the user by public_key. Sets userId, socketType, conn_id on the socket.
func (s *Server) authMiddleware(socket *socketio.Socket, next func(*socketio.ExtendedError)) {
	connID := uuid.NewString()
	socket.SetData(map[string]any{"conn_id": connID})

	auth, _ := socket.Handshake().Auth.(map[string]any)
	if auth == nil {
		slog.Warn("socket.auth.fail", "conn_id", connID, "reason", "missing_auth_object")
		next(socketio.NewExtendedError("Authentication required", nil))
		return
	}
	pub, _ := auth["publicKey"].(string)
	ts, _ := auth["timestamp"].(string)
	sig, _ := auth["signature"].(string)
	socketType, _ := auth["socketType"].(string)
	if socketType == "" {
		socketType = "app"
	}

	if pub == "" || ts == "" || sig == "" {
		slog.Warn("socket.auth.fail", "conn_id", connID, "reason", "missing_field")
		next(socketio.NewExtendedError("Authentication required", nil))
		return
	}

	tsInt, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		slog.Warn("socket.auth.fail", "conn_id", connID, "reason", "bad_timestamp")
		next(socketio.NewExtendedError("Authentication expired", nil))
		return
	}
	if abs64(time.Now().Unix()-tsInt) > 300 {
		slog.Warn("socket.auth.fail", "conn_id", connID, "reason", "expired", "skew_seconds", time.Now().Unix()-tsInt)
		next(socketio.NewExtendedError("Authentication expired", nil))
		return
	}

	if !crypto.VerifyTimestamp(pub, sig, ts) {
		slog.Warn("socket.auth.fail", "conn_id", connID, "reason", "bad_signature")
		next(socketio.NewExtendedError("Invalid signature", nil))
		return
	}

	row, err := s.d.Get(context.Background(), `SELECT id FROM users WHERE public_key = ?`, pub)
	if err != nil {
		slog.Error("socket.auth.db_error", "conn_id", connID, "err", err)
		next(socketio.NewExtendedError("Auth lookup failed", err))
		return
	}
	if row == nil {
		slog.Warn("socket.auth.fail", "conn_id", connID, "reason", "unknown_identity")
		next(socketio.NewExtendedError("Unknown identity", nil))
		return
	}
	userID := row.Str("id")

	socket.SetData(map[string]any{
		"conn_id":     connID,
		"user_id":     userID,
		"socket_type": socketType,
	})
	slog.Info("socket.auth.pass", "conn_id", connID, "user_id", userID, "socket_type", socketType)
	next(nil)
}

func (s *Server) onConnect(args ...any) {
	socket := args[0].(*socketio.Socket)
	data, _ := socket.Data().(map[string]any)
	userID, _ := data["user_id"].(string)
	socketType, _ := data["socket_type"].(string)
	connID, _ := data["conn_id"].(string)

	slog.Info("socket.connected",
		"conn_id", connID, "user_id", userID, "socket_type", socketType,
		"socket_id", string(socket.Id()))

	s.presence.Add(userID, socket, socketType)

	// Always register signaling handlers (service sockets need them too)
	s.registerSignalingHandlers(socket)

	if socketType != "service" {
		s.registerChatHandlers(socket)
		go s.deliverPending(socket, userID)
	}

	socket.On("disconnect", func(args ...any) { //nolint:errcheck
		reason := ""
		if len(args) > 0 {
			reason, _ = args[0].(string)
		}
		slog.Info("socket.disconnected",
			"conn_id", connID, "user_id", userID,
			"reason", reason, "socket_id", string(socket.Id()))
		s.presence.Remove(userID, socket)
		s.cleanupCallTracking(userID)
	})
}

// NotifyContactRequest implements Notifier.
func (s *Server) NotifyContactRequest(toUserID string, p ContactRequestPayload) {
	s.presence.EmitToUser(toUserID, "contact:request", p)
}

// NotifyContactAccepted implements Notifier.
func (s *Server) NotifyContactAccepted(toUserID string, p ContactAcceptedPayload) {
	s.presence.EmitToUser(toUserID, "contact:accepted", p)
}

// GetConnectionCounts implements ConnectionCounter.
func (s *Server) GetConnectionCounts() ConnectionCounts {
	return s.presence.GetConnectionCounts()
}

// abs64 because math.Abs only handles float64.
func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
