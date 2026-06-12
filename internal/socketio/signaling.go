package socketio

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/davin4u/faceless-server-go/internal/db"
	"github.com/davin4u/faceless-server-go/internal/stats"
	"github.com/google/uuid"
	socketio "github.com/zishang520/socket.io/v2/socket"
)

type callTiming struct {
	OfferTime  time.Time
	AnswerTime time.Time
	CallerID   string
	CalleeID   string
}

type iceBucket struct {
	Host  int `json:"host"`
	Srflx int `json:"srflx"`
	Relay int `json:"relay"`
	Prflx int `json:"prflx"`
}

type iceCounts struct {
	Caller iceBucket
	Callee iceBucket
}

var (
	callMu       sync.Mutex
	activeCalls  = map[string]time.Time{}   // callKey → answerTime (for duration)
	callTimings  = map[string]*callTiming{} // callKey → timing
	callIceCount = map[string]*iceCounts{}  // callKey → ICE counts per role
	candTypeRE   = regexp.MustCompile(`typ (\w+)`)
)

func callKey(a, b string) string {
	parts := []string{a, b}
	sort.Strings(parts)
	return parts[0] + ":" + parts[1]
}

// enqueueMissedCall inserts a call:missed pending event for the callee so they
// see a missed-call entry when they next reconnect (drained by delivery.go).
func enqueueMissedCall(ctx context.Context, d db.DB, from, to, callType string) error {
	payload, _ := json.Marshal(map[string]any{
		"from": from, "callType": callType, "timestamp": time.Now().Unix(),
	})
	_, err := d.Run(ctx,
		`INSERT INTO pending_events (id, user_id, event_type, payload, timestamp) VALUES (?, ?, 'call:missed', ?, ?)`,
		uuid.NewString(), to, string(payload), time.Now().Unix())
	return err
}

func (s *Server) registerSignalingHandlers(socket *socketio.Socket) {
	data, _ := socket.Data().(map[string]any)
	userID, _ := data["user_id"].(string)
	socketType, _ := data["socket_type"].(string)
	connID, _ := data["conn_id"].(string)
	ctx := context.Background()

	socket.On("call:offer", func(args ...any) {
		var p struct {
			To       string `json:"to"`
			SDP      string `json:"sdp"`
			CallType string `json:"callType"`
		}
		if !decodeArg(args, &p) || p.To == "" || p.SDP == "" {
			slog.Warn("signaling.call_offer.bad_payload", "conn_id", connID, "from", userID, "to", p.To)
			return
		}
		if p.CallType == "" {
			p.CallType = "voice"
		}

		slog.Info("signaling.call_offer.received",
			"from", userID, "to", p.To, "via_socket_type", socketType,
			"call_type", p.CallType, "sdp_bytes", len(p.SDP))

		appCount := s.presence.appSocketCount(p.To)
		serviceCount := s.presence.serviceSocketCount(p.To)

		// Callee has no foreground app socket (fully offline, or only a background
		// service socket). Wake them via FCM and hold the caller ringing until an
		// app socket appears, bounded by the call-timeout window.
		if !s.presence.HasAppSocket(p.To) {
			slog.Info("signaling.call_offer.waking",
				"from", userID, "to", p.To, "app_sockets", appCount, "service_sockets", serviceCount)
			go s.push.SendCallWake(context.Background(), p.To, userID, p.CallType)
			if !s.presence.WaitForAppSocket(ctx, p.To, 25*time.Second) {
				slog.Info("signaling.call_offer.unavailable",
					"from", userID, "to", p.To, "reason", "wake_timeout")
				if err := enqueueMissedCall(ctx, s.d, userID, p.To, p.CallType); err != nil {
					slog.Error("signaling.missed_call.enqueue_error", "to", p.To, "err", err)
				}
				socket.Emit("call:unavailable", map[string]any{})
				return
			}
			slog.Info("signaling.call_offer.woke", "from", userID, "to", p.To)
			// fall through: the callee now has an app socket; forward the offer below.
		}

		// Look up caller display name
		callerName := "Unknown"
		if row, _ := s.d.Get(ctx, `SELECT display_name FROM users WHERE id = ?`, userID); row != nil {
			callerName = row.Str("display_name")
		}

		s.presence.EmitToUser(p.To, "call:offer", map[string]any{
			"from": userID, "sdp": p.SDP,
			"callType": p.CallType, "callerName": callerName,
		})
		slog.Info("signaling.call_offer.forwarded",
			"from", userID, "to", p.To, "via_socket_type", socketType,
			"call_type", p.CallType, "sdp_bytes", len(p.SDP),
			"app_sockets", appCount, "service_sockets", serviceCount,
			"caller_name", callerName)

		k := callKey(userID, p.To)
		callMu.Lock()
		callTimings[k] = &callTiming{OfferTime: time.Now(), CallerID: userID, CalleeID: p.To}
		callIceCount[k] = &iceCounts{}
		callMu.Unlock()

		col := stats.ColAudioCalls
		if p.CallType == "video" {
			col = stats.ColVideoCalls
		}
		go func() { _ = stats.IncrementDaily(ctx, s.d, col, 1) }()
	})

	socket.On("call:answer", func(args ...any) {
		var p struct {
			To  string `json:"to"`
			SDP string `json:"sdp"`
		}
		if !decodeArg(args, &p) || p.To == "" || p.SDP == "" {
			return
		}
		k := callKey(userID, p.To)
		callMu.Lock()
		t := callTimings[k]
		if t != nil {
			t.AnswerTime = time.Now()
		}
		activeCalls[k] = time.Now()
		callMu.Unlock()

		s.presence.EmitToUser(p.To, "call:answer", map[string]any{
			"from": userID, "sdp": p.SDP,
		})
		elapsedMs := int64(0)
		if t != nil {
			elapsedMs = time.Since(t.OfferTime).Milliseconds()
		}
		slog.Info("signaling.call_answer.forwarded",
			"from", userID, "to", p.To, "sdp_bytes", len(p.SDP),
			"elapsed_since_offer_ms", elapsedMs)
	})

	socket.On("call:ice", func(args ...any) {
		var p struct {
			To        string                 `json:"to"`
			Candidate map[string]interface{} `json:"candidate"`
		}
		if !decodeArg(args, &p) || p.To == "" || p.Candidate == nil {
			return
		}
		candStr, _ := p.Candidate["candidate"].(string)
		ct := "unknown"
		if m := candTypeRE.FindStringSubmatch(candStr); len(m) == 2 {
			ct = m[1]
		}

		k := callKey(userID, p.To)
		callMu.Lock()
		ice := callIceCount[k]
		t := callTimings[k]
		role := "callee"
		if t != nil && t.CallerID == userID {
			role = "caller"
		}
		if ice != nil {
			b := &ice.Callee
			if role == "caller" {
				b = &ice.Caller
			}
			switch ct {
			case "host":
				b.Host++
			case "srflx":
				b.Srflx++
			case "relay":
				b.Relay++
			case "prflx":
				b.Prflx++
			}
		}
		callMu.Unlock()

		if s.logICE {
			slog.Info("signaling.call_ice",
				"from", userID, "to", p.To, "role", role, "candidate_type", ct,
				"candidate", candStr)
		} else {
			slog.Debug("signaling.call_ice",
				"from", userID, "to", p.To, "role", role, "candidate_type", ct)
		}

		s.presence.EmitToUser(p.To, "call:ice", map[string]any{
			"from": userID, "candidate": p.Candidate,
		})
	})

	socket.On("call:hangup", func(args ...any) {
		var p struct {
			To string `json:"to"`
		}
		if !decodeArg(args, &p) || p.To == "" {
			return
		}
		k := callKey(userID, p.To)
		s.presence.EmitToUser(p.To, "call:hangup", map[string]any{"from": userID})
		s.endCall(k, "hangup", userID)
	})

	socket.On("call:reject", func(args ...any) {
		var p struct {
			To string `json:"to"`
		}
		if !decodeArg(args, &p) || p.To == "" {
			return
		}
		k := callKey(userID, p.To)
		s.presence.EmitToUser(p.To, "call:reject", map[string]any{"from": userID})
		s.endCall(k, "reject", userID)
	})

	socket.On("call:toggle-video", func(args ...any) {
		var p struct {
			To           string `json:"to"`
			VideoEnabled *bool  `json:"videoEnabled"`
		}
		if !decodeArg(args, &p) || p.To == "" || p.VideoEnabled == nil {
			return
		}
		s.presence.EmitToUser(p.To, "call:toggle-video", map[string]any{
			"from": userID, "videoEnabled": *p.VideoEnabled,
		})
		slog.Info("signaling.call_toggle_video", "from", userID, "to", p.To, "video", *p.VideoEnabled)
	})
}

func (s *Server) endCall(k, reason, byUserID string) {
	ctx := context.Background()
	callMu.Lock()
	t := callTimings[k]
	ice := callIceCount[k]
	startTime, hadAnswer := activeCalls[k]
	delete(callTimings, k)
	delete(callIceCount, k)
	delete(activeCalls, k)
	callMu.Unlock()

	if t != nil {
		offerToAnswer := int64(-1)
		answerToEnd := int64(-1)
		if !t.AnswerTime.IsZero() {
			offerToAnswer = t.AnswerTime.Sub(t.OfferTime).Milliseconds()
			answerToEnd = time.Since(t.AnswerTime).Milliseconds()
		}
		fields := []any{
			"call_key", k,
			"caller_id", t.CallerID,
			"callee_id", t.CalleeID,
			"reason", reason,
			"ended_by", byUserID,
			"offer_to_answer_ms", offerToAnswer,
			"answer_to_end_ms", answerToEnd,
		}
		if ice != nil {
			fields = append(fields, "caller_ice", ice.Caller, "callee_ice", ice.Callee)
		}
		slog.Info("signaling.call.summary", fields...)
	}

	if hadAnswer {
		duration := int64(time.Since(startTime).Seconds())
		if duration > 0 {
			go func() {
				_ = stats.IncrementDaily(ctx, s.d, stats.ColCompletedCalls, 1)
				_ = stats.IncrementDaily(ctx, s.d, stats.ColCallDurationSecs, duration)
			}()
		}
	}
}

func (s *Server) cleanupCallTracking(userID string) {
	callMu.Lock()
	keys := make([]string, 0, len(callTimings))
	for k := range callTimings {
		if strings.Contains(k, userID) {
			keys = append(keys, k)
		}
	}
	callMu.Unlock()
	for _, k := range keys {
		s.endCall(k, "disconnect", userID)
	}
}
