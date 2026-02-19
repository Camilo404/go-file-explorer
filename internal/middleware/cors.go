package middleware

import (
	"net/http"

	"github.com/rs/cors"
)

func CORS(origins []string) func(http.Handler) http.Handler {
	if len(origins) == 0 {
		origins = []string{"*"}
	}

	handler := cors.New(cors.Options{
		AllowedOrigins:   origins,
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "X-Request-ID"},
		ExposedHeaders:   []string{"Content-Disposition", "Content-Length", "X-Request-ID"},
		MaxAge:           3600,
		AllowCredentials: false,
	})

	return handler.Handler
}
