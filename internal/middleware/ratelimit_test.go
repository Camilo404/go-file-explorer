package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRateLimitMiddleware_UnlimitedGeneral(t *testing.T) {
	// Setup middleware with generalRPM = 0 (unlimited) and authRPM = 1
	mw := NewRateLimitMiddleware(0, 1)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := mw.Handler(nextHandler)

	// Make many requests to a general endpoint
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "/api/v1/files", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Request %d failed with status %d", i, rec.Code)
		}
	}
}

func TestRateLimitMiddleware_LimitedAuth(t *testing.T) {
	// Setup middleware with generalRPM = 0 (unlimited) and authRPM = 1
	mw := NewRateLimitMiddleware(0, 1)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := mw.Handler(nextHandler)

	// First request to auth endpoint should succeed
	req1 := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusOK, rec1.Code)

	// Second request should be rate limited (since limit is 1 per minute)
	// Note: rate.Limiter allows bursts. NewLimiter(Every(1m), 1) has burst 1.
	// So 1st request consumes 1 token. 2nd request should fail immediately if we don't wait.

	req2 := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	// With burst 1, the first request consumes the token. The second request comes immediately.
	// It should be 429.
	assert.Equal(t, http.StatusTooManyRequests, rec2.Code)
}

func TestRateLimitMiddleware_Configuration(t *testing.T) {
	mw := NewRateLimitMiddleware(-1, 0)
	assert.Equal(t, -1, mw.generalRPM)
	assert.Equal(t, 10, mw.authRPM) // Default fallback for auth
}
