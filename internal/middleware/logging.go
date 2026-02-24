package middleware

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"
)

const requestIDHeader = "X-Request-ID"

// errorBody is a minimal struct used to extract error details from JSON responses.
type errorBody struct {
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details string `json:"details"`
	} `json:"error"`
}

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

		duration := time.Since(started).Milliseconds()

		attrs := []any{
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.status,
			"duration_ms", duration,
			"client_ip", r.RemoteAddr,
		}

		// Add query string for error responses to help reproduce issues.
		if wrapped.status >= 400 && r.URL.RawQuery != "" {
			attrs = append(attrs, "query", r.URL.RawQuery)
		}

		// For error responses, extract and attach error details from the body.
		if wrapped.status >= 400 && wrapped.body.Len() > 0 {
			var parsed errorBody
			if err := json.Unmarshal(wrapped.body.Bytes(), &parsed); err == nil && parsed.Error != nil {
				attrs = append(attrs, "error_code", parsed.Error.Code)
				attrs = append(attrs, "error_message", parsed.Error.Message)
				if parsed.Error.Details != "" {
					attrs = append(attrs, "error_details", parsed.Error.Details)
				}
			}
		}

		switch {
		case wrapped.status >= 500:
			slog.Error("request", attrs...)
		case wrapped.status >= 400:
			slog.Warn("request", attrs...)
		default:
			slog.Info("request", attrs...)
		}
	})
}

type responseWriter struct {
	http.ResponseWriter
	status      int
	body        bytes.Buffer
	wroteHeader bool
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	if rw.wroteHeader {
		return
	}
	rw.status = statusCode
	rw.wroteHeader = true
	rw.ResponseWriter.WriteHeader(statusCode)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	// Capture the body only for error responses so we can log error details.
	if rw.status >= 400 {
		rw.body.Write(b)
	}
	return rw.ResponseWriter.Write(b)
}

func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response writer does not support hijacking")
	}
	return hijacker.Hijack()
}
