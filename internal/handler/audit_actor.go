package handler

import (
	"net"
	"net/http"
	"strings"

	"go-file-explorer/internal/middleware"
	"go-file-explorer/internal/model"
)

func actorFromRequest(r *http.Request) model.AuditActor {
	actor := model.AuditActor{IP: clientIP(r)}

	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		return actor
	}

	actor.UserID = claims.UserID
	actor.Username = claims.Username
	actor.Role = claims.Role

	return actor
}

func clientIP(r *http.Request) string {
	xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return ip
			}
		}
	}

	xri := strings.TrimSpace(r.Header.Get("X-Real-IP"))
	if xri != "" {
		return xri
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}

	return strings.TrimSpace(r.RemoteAddr)
}
