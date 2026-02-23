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

func TestAuthFlowAndProtectedEndpoints(t *testing.T) {
	store, err := storage.New(t.TempDir())
	require.NoError(t, err)

	server, accessToken, refreshToken := newAuthedServer(t, store)
	t.Cleanup(server.Close)

	meResp := doAuthRequest(t, http.MethodGet, server.URL+"/api/v1/auth/me", accessToken)
	t.Cleanup(func() { _ = meResp.Body.Close() })
	require.Equal(t, http.StatusOK, meResp.StatusCode)

	refreshPayload, err := json.Marshal(map[string]string{"refresh_token": refreshToken})
	require.NoError(t, err)
	refreshResp, err := http.Post(server.URL+"/api/v1/auth/refresh", "application/json", bytes.NewReader(refreshPayload))
	require.NoError(t, err)
	t.Cleanup(func() { _ = refreshResp.Body.Close() })
	require.Equal(t, http.StatusOK, refreshResp.StatusCode)

	protectedResp := doRequest(t, mustNewRequest(t, http.MethodGet, server.URL+"/api/v1/files", nil))
	t.Cleanup(func() { _ = protectedResp.Body.Close() })
	require.Equal(t, http.StatusUnauthorized, protectedResp.StatusCode)
}

func TestAdminCanRegisterUser(t *testing.T) {
	store, err := storage.New(t.TempDir())
	require.NoError(t, err)

	server, accessToken, _ := newAuthedServer(t, store)
	t.Cleanup(server.Close)

	registerPayload, err := json.Marshal(map[string]string{
		"username": "editor1",
		"password": "Password123!",
		"role":     "editor",
	})
	require.NoError(t, err)

	registerResp := doAuthJSONRequest(t, http.MethodPost, server.URL+"/api/v1/auth/register", registerPayload, accessToken)
	t.Cleanup(func() { _ = registerResp.Body.Close() })
	require.Equal(t, http.StatusCreated, registerResp.StatusCode)
}
