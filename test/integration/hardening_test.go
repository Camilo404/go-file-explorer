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

func TestSecurityHeadersOnResponses(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	store, err := storage.New(t.TempDir())
	require.NoError(t, err)

	usersFile := filepath.Join(t.TempDir(), "users.json")
	authService, err := service.NewAuthService(usersFile, "test-secret", 15*time.Minute, 24*time.Hour)
	require.NoError(t, err)
	authMiddleware := middleware.NewAuthMiddleware(authService)
	authHandler := handler.NewAuthHandler(authService)

	directoryService := service.NewDirectoryService(store)
	directoryHandler := handler.NewDirectoryHandler(directoryService)
	fileService := service.NewFileService(store, nil)
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
		RateLimitRPM:       100,
		AuthRateLimitRPM:   2,
		MaxUploadSize:      10 * 1024 * 1024,
		SearchMaxDepth:     10,
		SearchTimeout:      30 * time.Second,
		TrashRoot:          trashRoot,
		TrashIndexFile:     trashIndexFile,
		AuditLogFile:       auditLogFile,
	}

	server := httptest.NewServer(router.New(cfg, authMiddleware, authHandler, directoryHandler, fileHandler, operationsHandler, searchHandler))
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
