//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go-file-explorer/internal/config"
	"go-file-explorer/internal/handler"
	"go-file-explorer/internal/middleware"
	"go-file-explorer/internal/router"
	"go-file-explorer/internal/service"
	"go-file-explorer/internal/storage"
)

func newAuthedServer(t *testing.T, store *storage.Storage) (*httptest.Server, string, string) {
	t.Helper()

	usersFile := filepath.Join(t.TempDir(), "users.json")
	authService, err := service.NewAuthService(usersFile, "test-secret", 15*time.Minute, 24*time.Hour)
	require.NoError(t, err)
	authMiddleware := middleware.NewAuthMiddleware(authService)
	authHandler := handler.NewAuthHandler(authService)

	directoryService := service.NewDirectoryService(store)
	directoryHandler := handler.NewDirectoryHandler(directoryService)
	thumbnailRoot := filepath.Join(t.TempDir(), "thumbnails")
	fileService := service.NewFileService(store, nil, thumbnailRoot)
	fileHandler := handler.NewFileHandler(fileService, 10*1024*1024)
	trashRoot := filepath.Join(t.TempDir(), "trash")
	trashIndexFile := filepath.Join(t.TempDir(), "trash-index.json")
	auditLogFile := filepath.Join(t.TempDir(), "audit.log")

	trashService, err := service.NewTrashService(store, trashRoot, trashIndexFile)
	require.NoError(t, err)
	auditService, err := service.NewAuditService(auditLogFile)
	require.NoError(t, err)

	operationsService := service.NewOperationsService(store, trashService, auditService)
	operationsHandler := handler.NewOperationsHandler(operationsService)
	searchService := service.NewSearchService(store, 10, 30*time.Second)
	searchHandler := handler.NewSearchHandler(searchService)

	cfg := &config.Config{
		ServerPort:         "8080",
		ServerReadTimeout:  15 * time.Second,
		ServerWriteTimeout: 30 * time.Second,
		ServerIdleTimeout:  120 * time.Second,
		StorageRoot:        store.RootAbs(),
		JWTSecret:          "test-secret",
		JWTAccessTTL:       15 * time.Minute,
		JWTRefreshTTL:      24 * time.Hour,
		UsersFile:          usersFile,
		CORSOrigins:        []string{"*"},
		RateLimitRPM:       1000,
		AuthRateLimitRPM:   1000,
		MaxUploadSize:      10 * 1024 * 1024,
		SearchMaxDepth:     10,
		SearchTimeout:      30 * time.Second,
		TrashRoot:          trashRoot,
		TrashIndexFile:     trashIndexFile,
		AuditLogFile:       auditLogFile,
		ThumbnailRoot:      thumbnailRoot,
	}

	server := httptest.NewServer(router.New(cfg, authMiddleware, authHandler, directoryHandler, fileHandler, operationsHandler, searchHandler))

	loginPayload := map[string]string{"username": "admin", "password": "admin123"}
	body, err := json.Marshal(loginPayload)
	require.NoError(t, err)

	resp, err := http.Post(server.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var parsed struct {
		Success bool `json:"success"`
		Data    struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&parsed))
	require.True(t, parsed.Success)
	require.NotEmpty(t, parsed.Data.AccessToken)
	require.NotEmpty(t, parsed.Data.RefreshToken)

	return server, parsed.Data.AccessToken, parsed.Data.RefreshToken
}

func newAuthRequest(t *testing.T, method string, url string, body []byte, accessToken string) *http.Request {
	t.Helper()

	var payloadReader *bytes.Reader
	if body == nil {
		payloadReader = bytes.NewReader([]byte{})
	} else {
		payloadReader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, url, payloadReader)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return req
}

func doRequest(t *testing.T, req *http.Request) *http.Response {
	t.Helper()

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func doAuthJSONRequest(t *testing.T, method string, url string, body []byte, accessToken string) *http.Response {
	t.Helper()

	req := newAuthRequest(t, method, url, body, accessToken)
	return doRequest(t, req)
}

func doAuthRequest(t *testing.T, method string, url string, accessToken string) *http.Response {
	t.Helper()

	req := newAuthRequest(t, method, url, nil, accessToken)
	return doRequest(t, req)
}

func mustNewRequest(t *testing.T, method string, url string, body []byte) *http.Request {
	t.Helper()

	var payloadReader *bytes.Reader
	if body == nil {
		payloadReader = bytes.NewReader([]byte{})
	} else {
		payloadReader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, url, payloadReader)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return req
}
