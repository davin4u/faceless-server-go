package routes

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/davin4u/faceless-server-go/internal/auth"
	"github.com/davin4u/faceless-server-go/internal/files"
	"github.com/go-chi/chi/v5"
)

// NewFiles returns a chi router for /api/files/*. It assumes
// auth.RequireSignatureAuth has already populated the context.
func NewFiles(svc *files.Service) http.Handler {
	r := chi.NewRouter()
	r.Post("/request-upload", requestUpload(svc))
	r.Post("/commit", commitFile(svc))
	r.Get("/{fileId}/download-url", downloadURL(svc))
	return r
}

func requestUpload(svc *files.Service) http.HandlerFunc {
	type req struct {
		SizeBytes int64  `json:"sizeBytes"`
		To        string `json:"to"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		u := auth.UserFromCtx(r.Context())
		if u == nil {
			writeJSONErr(w, 401, "Unauthorized")
			return
		}
		var b req
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil || b.To == "" || b.SizeBytes <= 0 {
			writeJSONErr(w, 400, "sizeBytes and to are required")
			return
		}
		fileID, url, err := svc.RequestUpload(r.Context(), u.ID, b.To, b.SizeBytes)
		switch {
		case errors.Is(err, files.ErrNotContacts):
			writeJSONErr(w, 403, "Recipient is not in your contacts")
			return
		case errors.Is(err, files.ErrTooLarge):
			writeJSONErr(w, 413, "File exceeds the size limit")
			return
		case errors.Is(err, files.ErrStorageFull):
			writeJSON(w, 507, map[string]any{"error": "Storage is full", "code": "storage_full"})
			return
		case err != nil:
			writeJSONErr(w, 500, "Failed to reserve upload")
			return
		}
		writeJSON(w, 200, map[string]string{"fileId": fileID, "uploadUrl": url})
	}
}

func commitFile(svc *files.Service) http.HandlerFunc {
	type req struct {
		FileID    string `json:"fileId"`
		MessageID string `json:"messageId"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		u := auth.UserFromCtx(r.Context())
		if u == nil {
			writeJSONErr(w, 401, "Unauthorized")
			return
		}
		var b req
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil || b.FileID == "" || b.MessageID == "" {
			writeJSONErr(w, 400, "fileId and messageId are required")
			return
		}
		err := svc.Commit(r.Context(), b.FileID, u.ID, b.MessageID)
		switch {
		case errors.Is(err, files.ErrNotFound):
			writeJSONErr(w, 404, "Upload not found")
			return
		case errors.Is(err, files.ErrSizeMismatch):
			writeJSONErr(w, 400, "Uploaded size mismatch")
			return
		case err != nil:
			writeJSONErr(w, 500, "Failed to commit upload")
			return
		}
		writeJSON(w, 200, map[string]string{"status": "committed"})
	}
}

func downloadURL(svc *files.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := auth.UserFromCtx(r.Context())
		if u == nil {
			writeJSONErr(w, 401, "Unauthorized")
			return
		}
		fileID := chi.URLParam(r, "fileId")
		url, err := svc.DownloadURL(r.Context(), fileID, u.ID)
		switch {
		case errors.Is(err, files.ErrNotFound):
			writeJSONErr(w, 404, "File not found")
			return
		case errors.Is(err, files.ErrForbidden):
			writeJSONErr(w, 403, "Forbidden")
			return
		case err != nil:
			writeJSONErr(w, 500, "Failed to sign download URL")
			return
		}
		writeJSON(w, 200, map[string]string{"url": url})
	}
}
