package middleware

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"go-file-explorer/internal/model"
)

type clientLimiter struct {
	general  *rate.Limiter
	auth     *rate.Limiter
	lastSeen time.Time
}

type RateLimitMiddleware struct {
	generalRPM int
	authRPM    int
	mu         sync.Mutex
	clients    map[string]*clientLimiter
}

func NewRateLimitMiddleware(generalRPM int, authRPM int) *RateLimitMiddleware {
	if generalRPM <= 0 {
		generalRPM = 100
	}
	if authRPM <= 0 {
		authRPM = 10
	}

	return &RateLimitMiddleware{
		generalRPM: generalRPM,
		authRPM:    authRPM,
		clients:    map[string]*clientLimiter{},
	}
}

func (m *RateLimitMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(strings.ToLower(r.URL.Path), "/api/v1/files/thumbnail") {
			next.ServeHTTP(w, r)
			return
		}

		clientIP := extractClientIP(r)
		limiter := m.getLimiter(clientIP)

		target := limiter.general
		if strings.HasPrefix(strings.ToLower(r.URL.Path), "/api/v1/auth") {
			target = limiter.auth
		}

		if !target.Allow() {
			w.Header().Set("Retry-After", "60")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(model.APIResponse{
				Success: false,
				Error: &model.APIError{
					Code:    "RATE_LIMITED",
					Message: "Too many requests",
				},
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (m *RateLimitMiddleware) getLimiter(clientIP string) *clientLimiter {
	m.mu.Lock()
	defer m.mu.Unlock()

	if limiter, exists := m.clients[clientIP]; exists {
		limiter.lastSeen = time.Now()
		m.gcLocked()
		return limiter
	}

	general := rate.NewLimiter(rate.Every(time.Minute/time.Duration(m.generalRPM)), m.generalRPM)
	auth := rate.NewLimiter(rate.Every(time.Minute/time.Duration(m.authRPM)), m.authRPM)
	created := &clientLimiter{general: general, auth: auth, lastSeen: time.Now()}
	m.clients[clientIP] = created
	m.gcLocked()

	return created
}

func (m *RateLimitMiddleware) gcLocked() {
	if len(m.clients) < 1000 {
		return
	}

	cutoff := time.Now().Add(-10 * time.Minute)
	for ip, limiter := range m.clients {
		if limiter.lastSeen.Before(cutoff) {
			delete(m.clients, ip)
		}
	}
}

func extractClientIP(r *http.Request) string {
	forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}

	realIP := strings.TrimSpace(r.Header.Get("X-Real-IP"))
	if realIP != "" {
		return realIP
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}

	if strings.TrimSpace(r.RemoteAddr) == "" {
		return "unknown"
	}

	return r.RemoteAddr
}
