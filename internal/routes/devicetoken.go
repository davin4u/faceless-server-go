package routes

import (
	"encoding/json"
	"net/http"

	"github.com/davin4u/faceless-server-go/internal/auth"
	"github.com/davin4u/faceless-server-go/internal/db"
)

// DeviceToken handles POST and DELETE /api/device-token.
type DeviceToken struct{ d db.DB }

// NewDeviceToken returns a new DeviceToken handler.
func NewDeviceToken(d db.DB) *DeviceToken { return &DeviceToken{d: d} }

func (h *DeviceToken) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromCtx(r.Context())
	if u == nil {
		writeJSONErr(w, 401, "Unauthorized")
		return
	}
	var body struct {
		Token    string `json:"token"`
		Platform string `json:"platform"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Token == "" {
		writeJSONErr(w, 400, "token required")
		return
	}
	platform := body.Platform
	if platform == "" {
		platform = "android"
	}
	switch r.Method {
	case http.MethodPost:
		if err := db.UpsertDeviceToken(r.Context(), h.d, u.ID, body.Token, platform); err != nil {
			writeJSONErr(w, 500, "db error")
			return
		}
	case http.MethodDelete:
		if err := db.DeleteDeviceToken(r.Context(), h.d, u.ID, body.Token); err != nil {
			writeJSONErr(w, 500, "db error")
			return
		}
	default:
		writeJSONErr(w, 405, "method not allowed")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
