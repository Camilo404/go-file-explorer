package middleware

import (
	"net/http"
	"time"
)

func Timeout(timeout time.Duration) func(http.Handler) http.Handler {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	message := `{"success":false,"error":{"code":"REQUEST_TIMEOUT","message":"request timed out"}}`

	return func(next http.Handler) http.Handler {
		return http.TimeoutHandler(next, timeout, message)
	}
}
