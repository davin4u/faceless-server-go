package logger

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type ctxKey string

const (
	requestIDKey ctxKey = "request_id"
	loggerKey    ctxKey = "logger"
)

func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := uuid.NewString()
		log := slog.With(
			"request_id", reqID,
			"method", r.Method,
			"path", r.URL.Path,
			"remote_addr", r.RemoteAddr,
		)

		log.Info("http.request.start")
		start := time.Now()

		sw := &statusWriter{ResponseWriter: w, status: 200}
		ctx := context.WithValue(r.Context(), requestIDKey, reqID)
		ctx = context.WithValue(ctx, loggerKey, log)
		next.ServeHTTP(sw, r.WithContext(ctx))

		log.Info("http.request.end",
			"status", sw.status,
			"duration_ms", float64(time.Since(start).Microseconds())/1000.0,
			"bytes", sw.bytes,
		)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(b []byte) (int, error) {
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

// FromCtx returns a logger pre-seeded with request_id, falling back to the default.
func FromCtx(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

// RequestID extracts the request ID from a context (empty string if missing).
func RequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}
