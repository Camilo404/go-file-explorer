package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

const requestIDHeader = "X-Request-ID"

func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get(requestIDHeader)
		if requestID == "" {
			requestID = uuid.NewString()
		}

		w.Header().Set(requestIDHeader, requestID)

		started := time.Now()
		wrapped := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		slog.Info("request",
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.status,
			"duration_ms", time.Since(started).Milliseconds(),
			"client_ip", r.RemoteAddr,
		)
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.status = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}
