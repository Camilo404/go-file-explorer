package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"go-file-explorer/internal/model"
)

type tokenValidator interface {
	ValidateToken(tokenString string, expectedType string) (*model.AuthClaims, error)
}

type contextKey string

const authClaimsContextKey contextKey = "auth_claims"

type AuthMiddleware struct {
	validator tokenValidator
}

func NewAuthMiddleware(validator tokenValidator) *AuthMiddleware {
	return &AuthMiddleware{validator: validator}
}

func (m *AuthMiddleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		if header == "" || !strings.HasPrefix(strings.ToLower(header), "bearer ") {
			writeUnauthorized(w, "UNAUTHORIZED", "missing or invalid authorization header")
			return
		}

		token := strings.TrimSpace(header[7:])
		claims, err := m.validator.ValidateToken(token, "access")
		if err != nil {
			writeUnauthorized(w, "UNAUTHORIZED", "invalid or expired token")
			return
		}

		ctx := context.WithValue(r.Context(), authClaimsContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *AuthMiddleware) RequireRoles(allowedRoles ...string) func(http.Handler) http.Handler {
	roleSet := map[string]struct{}{}
	for _, role := range allowedRoles {
		roleSet[strings.ToLower(strings.TrimSpace(role))] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFromContext(r.Context())
			if !ok {
				writeUnauthorized(w, "UNAUTHORIZED", "authentication required")
				return
			}

			if _, exists := roleSet[strings.ToLower(claims.Role)]; !exists {
				writeUnauthorized(w, "FORBIDDEN", "insufficient permissions")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func ClaimsFromContext(ctx context.Context) (*model.AuthClaims, bool) {
	claims, ok := ctx.Value(authClaimsContextKey).(*model.AuthClaims)
	return claims, ok
}

func writeUnauthorized(w http.ResponseWriter, code string, message string) {
	w.Header().Set("Content-Type", "application/json")
	if code == "FORBIDDEN" {
		w.WriteHeader(http.StatusForbidden)
	} else {
		w.WriteHeader(http.StatusUnauthorized)
	}

	_ = json.NewEncoder(w).Encode(model.APIResponse{
		Success: false,
		Error: &model.APIError{
			Code:    code,
			Message: message,
		},
	})
}
