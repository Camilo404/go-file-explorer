package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go-file-explorer/internal/config"
	"go-file-explorer/internal/database"
	"go-file-explorer/internal/handler"
	"go-file-explorer/internal/middleware"
	"go-file-explorer/internal/repository"
	"go-file-explorer/internal/router"
	"go-file-explorer/internal/service"
	"go-file-explorer/internal/storage"
	"net/http"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	store, err := storage.New(cfg.StorageRoot)
	if err != nil {
		slog.Error("failed to initialize storage", "error", err)
		os.Exit(1)
	}

	// ── PostgreSQL ───────────────────────────────────────────────────
	slog.Info("connecting to PostgreSQL")
	db, err := database.New(context.Background(), cfg.DatabaseURL, cfg.DBMaxConns, cfg.DBMinConns)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.EnsureSchema(context.Background()); err != nil {
		slog.Error("failed to ensure database schema", "error", err)
		os.Exit(1)
	}

	pool := db.Pool
	userRepo := repository.NewUserRepository(pool)
	tokenRepo := repository.NewTokenRepository(pool)
	auditRepo := repository.NewAuditRepository(pool)
	shareRepo := repository.NewShareRepository(pool)
	trashRepo := repository.NewTrashRepository(pool)
	jobRepo := repository.NewJobRepository(pool)
	slog.Info("database ready")

	// ── Auth service ─────────────────────────────────────────────────
	authService, err := service.NewAuthService(cfg.JWTSecret, cfg.JWTAccessTTL, cfg.JWTRefreshTTL, userRepo, tokenRepo)
	if err != nil {
		slog.Error("failed to initialize auth service", "error", err)
		os.Exit(1)
	}
	authMiddleware := middleware.NewAuthMiddleware(authService)
	authHandler := handler.NewAuthHandler(authService)

	directoryService := service.NewDirectoryService(store)
	directoryHandler := handler.NewDirectoryHandler(directoryService)
	fileService := service.NewFileService(store, cfg.AllowedMIMETypes, cfg.ThumbnailRoot)
	fileHandler := handler.NewFileHandler(fileService, cfg.MaxUploadSize)
	trashService, err := service.NewTrashService(store, cfg.TrashRoot, trashRepo)
	if err != nil {
		slog.Error("failed to initialize trash service", "error", err)
		os.Exit(1)
	}
	trashService.SetThumbnailRoot(cfg.ThumbnailRoot)
	auditService := service.NewAuditService(auditRepo)
	auditHandler := handler.NewAuditHandler(auditService)
	docsHandler := handler.NewDocsHandler("./docs/openapi.yaml")
	operationsService := service.NewOperationsService(store, trashService, auditService)
	operationsHandler := handler.NewOperationsHandler(operationsService)
	jobService := service.NewJobService(operationsService, jobRepo)
	jobsHandler := handler.NewJobsHandler(jobService)
	searchService := service.NewSearchService(store, cfg.SearchMaxDepth, cfg.SearchTimeout)
	searchHandler := handler.NewSearchHandler(searchService)
	userHandler := handler.NewUserHandler(authService)
	storageHandler := handler.NewStorageHandler(store, []string{cfg.TrashRoot, cfg.ThumbnailRoot})
	shareService := service.NewShareService(shareRepo)
	shareHandler := handler.NewShareHandler(shareService, fileService)
	chunkedUploadService, err := service.NewChunkedUploadService(store, cfg.ChunkTempDir, cfg.AllowedMIMETypes)
	if err != nil {
		slog.Error("failed to initialize chunked upload service", "error", err)
		os.Exit(1)
	}
	chunkedUploadHandler := handler.NewChunkedUploadHandler(chunkedUploadService, cfg.ChunkMaxSize)
	appRouter := router.New(cfg, authMiddleware, authHandler, directoryHandler, fileHandler, operationsHandler, searchHandler, auditHandler, jobsHandler, docsHandler, userHandler, storageHandler, shareHandler, chunkedUploadHandler)

	// ── Chunked upload cleanup goroutine ─────────────────────────
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	go chunkedUploadService.StartCleanupTicker(cleanupCtx, cfg.ChunkExpiry)

	server := &http.Server{
		Addr:              ":" + cfg.ServerPort,
		Handler:           appRouter,
		ReadHeaderTimeout: cfg.ServerReadHeaderTimeout,
		WriteTimeout:      cfg.ServerWriteTimeout,
		IdleTimeout:       cfg.ServerIdleTimeout,
	}

	go func() {
		slog.Info("server starting", "addr", server.Addr)
		if serveErr := server.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			slog.Error("server failed", "error", serveErr)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cleanupCancel() // stop the chunk cleanup goroutine

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}

	slog.Info("server stopped")
}
