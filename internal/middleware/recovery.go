package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"go-file-explorer/internal/model"
)

func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("panic recovered", "error", fmt.Sprintf("%v", recovered), "stack", string(debug.Stack()))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = jsonEncode(w, model.APIResponse{
					Success: false,
					Error: &model.APIError{
						Code:    "INTERNAL_ERROR",
						Message: "Unexpected server error",
					},
				})
			}
		}()

		next.ServeHTTP(w, r)
	})
}
