package routes

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/davin4u/faceless-server-go/internal/auth"
	"github.com/davin4u/faceless-server-go/internal/avatars"
	"github.com/go-chi/chi/v5"
)

// NewAvatars returns a chi router for /api/avatar/*. Assumes
// auth.RequireSignatureAuth has populated the context.
func NewAvatars(svc *avatars.Service) http.Handler {
	r := chi.NewRouter()
	r.Post("/request-upload", avatarRequestUpload(svc))
	r.Post("/commit", avatarCommit(svc))
	r.Get("/{avatarId}/download-url", avatarDownloadURL(svc))
	r.Delete("/custom", avatarDeleteCustom(svc))
	return r
}

func avatarRequestUpload(svc *avatars.Service) http.HandlerFunc {
	type req struct {
		Kind      string `json:"kind"`
		SizeBytes int64  `json:"sizeBytes"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		u := auth.UserFromCtx(r.Context())
		if u == nil {
			writeJSONErr(w, 401, "Unauthorized")
			return
		}
		var b req
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil || b.SizeBytes <= 0 {
			writeJSONErr(w, 400, "kind and sizeBytes are required")
			return
		}
		id, url, err := svc.RequestUpload(r.Context(), u.ID, b.Kind, b.SizeBytes)
		switch {
		case errors.Is(err, avatars.ErrBadKind):
			writeJSONErr(w, 400, "Invalid kind")
			return
		case errors.Is(err, avatars.ErrTooLarge):
			writeJSONErr(w, 413, "Avatar exceeds the size limit")
			return
		case err != nil:
			writeJSONErr(w, 500, "Failed to reserve upload")
			return
		}
		writeJSON(w, 200, map[string]string{"avatarId": id, "uploadUrl": url})
	}
}

func avatarCommit(svc *avatars.Service) http.HandlerFunc {
	type req struct {
		AvatarID string `json:"avatarId"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		u := auth.UserFromCtx(r.Context())
		if u == nil {
			writeJSONErr(w, 401, "Unauthorized")
			return
		}
		var b req
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil || b.AvatarID == "" {
			writeJSONErr(w, 400, "avatarId is required")
			return
		}
		err := svc.Commit(r.Context(), b.AvatarID, u.ID)
		switch {
		case errors.Is(err, avatars.ErrNotFound):
			writeJSONErr(w, 404, "Upload not found")
			return
		case errors.Is(err, avatars.ErrSizeMismatch):
			writeJSONErr(w, 400, "Uploaded size mismatch")
			return
		case err != nil:
			writeJSONErr(w, 500, "Failed to commit upload")
			return
		}
		writeJSON(w, 200, map[string]string{"status": "committed"})
	}
}

func avatarDownloadURL(svc *avatars.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := auth.UserFromCtx(r.Context())
		if u == nil {
			writeJSONErr(w, 401, "Unauthorized")
			return
		}
		id := chi.URLParam(r, "avatarId")
		url, err := svc.DownloadURL(r.Context(), id, u.ID)
		switch {
		case errors.Is(err, avatars.ErrNotFound):
			writeJSONErr(w, 404, "Avatar not found")
			return
		case errors.Is(err, avatars.ErrForbidden):
			writeJSONErr(w, 403, "Forbidden")
			return
		case err != nil:
			writeJSONErr(w, 500, "Failed to sign download URL")
			return
		}
		writeJSON(w, 200, map[string]string{"url": url})
	}
}

func avatarDeleteCustom(svc *avatars.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := auth.UserFromCtx(r.Context())
		if u == nil {
			writeJSONErr(w, 401, "Unauthorized")
			return
		}
		if err := svc.DeleteCustom(r.Context(), u.ID); err != nil {
			writeJSONErr(w, 500, "Failed to delete avatar")
			return
		}
		writeJSON(w, 200, map[string]string{"status": "deleted"})
	}
}
