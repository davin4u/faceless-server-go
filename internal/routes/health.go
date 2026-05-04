package routes

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func NewHealth() http.Handler {
	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok"})
	})
	return r
}
