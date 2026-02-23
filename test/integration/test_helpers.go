//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go-file-explorer/internal/config"
	"go-file-explorer/internal/database"
	"go-file-explorer/internal/event"
	"go-file-explorer/internal/handler"
	"go-file-explorer/internal/middleware"
	"go-file-explorer/internal/repository"
	"go-file-explorer/internal/router"
	"go-file-explorer/internal/service"
	"go-file-explorer/internal/storage"
	"go-file-explorer/internal/websocket"
)

func newTestServer(t *testing.T, store storage.Storage, authRateLimitRPM int) *httptest.Server {
	t.Helper()

	// Database setup
	dbURL := "postgres://explorer:explorer@localhost:5432/file_explorer?sslmode=disable"
	ctx := context.Background()

	db, err := database.New(ctx, dbURL, 5, 1)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// Ensure schema (migrations)
	err = db.EnsureSchema(ctx)
	require.NoError(t, err)

	// Reset database
	_, err = db.Pool.Exec(ctx, "TRUNCATE TABLE users, refresh_tokens, audit_entries, shares, trash_records, jobs, job_items RESTART IDENTITY CASCADE")
	require.NoError(t, err)

	// Repositories
	userRepo := repository.NewUserRepository(db.Pool)
	tokenRepo := repository.NewTokenRepository(db.Pool)
	trashRepo := repository.NewTrashRepository(db.Pool)
	auditRepo := repository.NewAuditRepository(db.Pool)
	jobRepo := repository.NewJobRepository(db.Pool)
	shareRepo := repository.NewShareRepository(db.Pool)

	// Event Bus
	bus := event.NewBus()

	// Services
	authService, err := service.NewAuthService("test-secret-must-be-at-least-32-chars-long", 15*time.Minute, 24*time.Hour, userRepo, tokenRepo)
	require.NoError(t, err)

	directoryService := service.NewDirectoryService(store, bus)

	thumbnailRoot := filepath.Join(t.TempDir(), "thumbnails")
	fileService := service.NewFileService(store, []string{}, thumbnailRoot, bus)

	trashRoot := filepath.Join(t.TempDir(), "trash")
	trashService, err := service.NewTrashService(store, trashRoot, trashRepo)
	require.NoError(t, err)

	auditService := service.NewAuditService(auditRepo)

	operationsService := service.NewOperationsService(store, trashService, auditService, bus)
	jobService := service.NewJobService(operationsService, jobRepo, bus)
	searchService := service.NewSearchService(store, 10, 30*time.Second)
	shareService := service.NewShareService(shareRepo)

	chunkTempDir := filepath.Join(t.TempDir(), "chunks")
	chunkedUploadService, err := service.NewChunkedUploadService(store, chunkTempDir, []string{}, bus)
	require.NoError(t, err)

	// Handlers
	authMiddleware := middleware.NewAuthMiddleware(authService)
	authHandler := handler.NewAuthHandler(authService)
	directoryHandler := handler.NewDirectoryHandler(directoryService)
	fileHandler := handler.NewFileHandler(fileService, 10*1024*1024)
	auditHandler := handler.NewAuditHandler(auditService)
	operationsHandler := handler.NewOperationsHandler(operationsService)
	jobsHandler := handler.NewJobsHandler(jobService)
	searchHandler := handler.NewSearchHandler(searchService)
	docsHandler := handler.NewDocsHandler(filepath.Join("..", "..", "docs", "openapi.yaml"))
	userHandler := handler.NewUserHandler(authService)
	storageHandler := handler.NewStorageHandler(store, []string{})
	shareHandler := handler.NewShareHandler(shareService, fileService)
	chunkedUploadHandler := handler.NewChunkedUploadHandler(chunkedUploadService, 5*1024*1024)
	hub := websocket.NewHub(bus)

	cfg := &config.Config{
		ServerPort:              "8080",
		ServerReadHeaderTimeout: 15 * time.Second,
		ServerWriteTimeout:      30 * time.Second,
		ServerIdleTimeout:       120 * time.Second,
		RequestTimeout:          30 * time.Second,
		TransferTimeout:         30 * time.Minute,
		TransferIdleTimeout:     5 * time.Minute,
		StorageRoot:             store.RootAbs(),
		JWTSecret:               "test-secret-must-be-at-least-32-chars-long",
		JWTAccessTTL:            15 * time.Minute,
		JWTRefreshTTL:           24 * time.Hour,
		CORSOrigins:             []string{"*"},
		AuthRateLimitRPM:        authRateLimitRPM,
		MaxUploadSize:           10 * 1024 * 1024,
		SearchMaxDepth:          10,
		SearchTimeout:           30 * time.Second,
		AllowedMIMETypes:        []string{"*"},
		TrashRoot:               trashRoot,
		ThumbnailRoot:           thumbnailRoot,
		ChunkTempDir:            chunkTempDir,
		ChunkMaxSize:            5 * 1024 * 1024,
		ChunkExpiry:             24 * time.Hour,
		DatabaseURL:             dbURL,
		DBMaxConns:              5,
		DBMinConns:              1,
	}

	r := router.New(
		cfg,
		authMiddleware,
		authHandler,
		directoryHandler,
		fileHandler,
		operationsHandler,
		searchHandler,
		auditHandler,
		jobsHandler,
		docsHandler,
		userHandler,
		storageHandler,
		shareHandler,
		chunkedUploadHandler,
		hub,
	)

	return httptest.NewServer(r)
}

func newAuthedServer(t *testing.T, store *storage.Storage) (*httptest.Server, string, string) {
	server := newTestServer(t, store, 1000)

	// Login
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

func mustNewRequest(t *testing.T, method, url string, body io.Reader) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	require.NoError(t, err)
	return req
}

func doRequest(t *testing.T, req *http.Request) *http.Response {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func doAuthRequest(t *testing.T, method, url, token string) *http.Response {
	t.Helper()
	req := mustNewRequest(t, method, url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	return doRequest(t, req)
}

func doAuthJSONRequest(t *testing.T, method, url string, body []byte, token string) *http.Response {
	t.Helper()
	req := mustNewRequest(t, method, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	return doRequest(t, req)
}
