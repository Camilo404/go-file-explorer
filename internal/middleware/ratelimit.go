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

// extractClientIP determines the real client IP address.
// Priority order (most trusted first):
//  1. Cf-Connecting-IP  — set by Cloudflare (trusted proxy, cannot be spoofed by end-user)
//  2. X-Real-IP         — set by trusted reverse proxies (nginx, Caddy)
//  3. X-Forwarded-For   — first entry (may be spoofed if not behind a trusted proxy)
//  4. r.RemoteAddr      — direct TCP peer (fallback)
//
// When behind cloudflared, Cf-Connecting-IP is the authoritative source.
func extractClientIP(r *http.Request) string {
	// 1. Cloudflare Tunnel / Cloudflare proxy
	if cfIP := strings.TrimSpace(r.Header.Get("Cf-Connecting-IP")); cfIP != "" {
		return cfIP
	}

	// 2. Trusted reverse proxy header
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}

	// 3. X-Forwarded-For (take the first/leftmost entry)
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 {
			if ip := strings.TrimSpace(parts[0]); ip != "" {
				return ip
			}
		}
	}

	// 4. Direct connection
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}

	if strings.TrimSpace(r.RemoteAddr) == "" {
		return "unknown"
	}

	return r.RemoteAddr
}
