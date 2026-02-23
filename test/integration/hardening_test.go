//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"go-file-explorer/internal/storage"
)

func TestSecurityHeadersOnResponses(t *testing.T) {
	store, err := storage.New(t.TempDir())
	require.NoError(t, err)

	server, accessToken, _ := newAuthedServer(t, store)
	t.Cleanup(server.Close)

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/files", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))
	require.Equal(t, "DENY", resp.Header.Get("X-Frame-Options"))
	require.Equal(t, "no-referrer", resp.Header.Get("Referrer-Policy"))
}

func TestAuthRateLimitReturns429(t *testing.T) {
	store, err := storage.New(t.TempDir())
	require.NoError(t, err)

	// Set rate limit to 2 RPM
	server := newTestServer(t, store, 2)
	t.Cleanup(server.Close)

	loginPayload, err := json.Marshal(map[string]string{"username": "admin", "password": "admin123"})
	require.NoError(t, err)

	for attempt := 0; attempt < 2; attempt++ {
		resp, reqErr := http.Post(server.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(loginPayload))
		require.NoError(t, reqErr)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	}

	resp, err := http.Post(server.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(loginPayload))
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	require.NotEmpty(t, resp.Header.Get("Retry-After"))
}
