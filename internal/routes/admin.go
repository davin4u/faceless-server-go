package routes

import (
	"net/http"
	"time"

	"github.com/davin4u/faceless-server-go/internal/auth"
	"github.com/davin4u/faceless-server-go/internal/db"
	"github.com/davin4u/faceless-server-go/internal/socketio"
	"github.com/go-chi/chi/v5"
)

func NewAdmin(d db.DB, secret string, conns socketio.ConnectionCounter) http.Handler {
	r := chi.NewRouter()
	r.With(auth.RequireAdminBearer(secret)).Get("/api/admin/stats", statsHandler(d, conns))
	return r
}

func statsHandler(d db.DB, conns socketio.ConnectionCounter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		now := time.Now().Unix()
		startOfToday := now - (now % 86400)
		sevenDaysAgo := now - 7*86400

		total, _ := d.Get(ctx, `SELECT COUNT(*) AS count FROM users`)
		today, _ := d.Get(ctx, `SELECT COUNT(*) AS count FROM users WHERE created_at >= ?`, startOfToday)
		week, _ := d.Get(ctx, `SELECT COUNT(*) AS count FROM users WHERE created_at >= ?`, sevenDaysAgo)
		undelivered, _ := d.Get(ctx, `SELECT COUNT(*) AS count FROM messages WHERE delivered = 0`)
		dailyRows, _ := d.All(ctx, `SELECT * FROM daily_stats ORDER BY date DESC LIMIT 90`)

		writeJSON(w, 200, map[string]any{
			"users": map[string]int64{
				"total":  total.Int("count"),
				"today":  today.Int("count"),
				"last7d": week.Int("count"),
			},
			"messages": map[string]int64{
				"undelivered": undelivered.Int("count"),
			},
			"connections": conns.GetConnectionCounts(),
			"system":      collectSystem(),
			"dailyStats":  dailyStatsRows(dailyRows),
		})
	}
}

func dailyStatsRows(rows []db.Row) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, map[string]any{
			"date":                        r.Str("date"),
			"messages_sent":               r.Int("messages_sent"),
			"audio_calls":                 r.Int("audio_calls"),
			"video_calls":                 r.Int("video_calls"),
			"completed_calls":             r.Int("completed_calls"),
			"total_call_duration_seconds": r.Int("total_call_duration_seconds"),
			"registrations":               r.Int("registrations"),
		})
	}
	return out
}
