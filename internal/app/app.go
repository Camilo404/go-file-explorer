package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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

type App struct {
	server       *http.Server
	db           *database.DB
	cleanupFuncs []func()
}

func New() (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	store, err := storage.New(cfg.StorageRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	slog.Info("connecting to PostgreSQL")
	db, err := database.New(context.Background(), cfg.DatabaseURL, cfg.DBMaxConns, cfg.DBMinConns)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := db.EnsureSchema(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ensure database schema: %w", err)
	}

	pool := db.Pool
	userRepo := repository.NewUserRepository(pool)
	tokenRepo := repository.NewTokenRepository(pool)
	auditRepo := repository.NewAuditRepository(pool)
	shareRepo := repository.NewShareRepository(pool)
	trashRepo := repository.NewTrashRepository(pool)
	jobRepo := repository.NewJobRepository(pool)
	slog.Info("database ready")

	authService, err := service.NewAuthService(cfg.JWTSecret, cfg.JWTAccessTTL, cfg.JWTRefreshTTL, userRepo, tokenRepo)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize auth service: %w", err)
	}
	authMiddleware := middleware.NewAuthMiddleware(authService)
	authHandler := handler.NewAuthHandler(authService)

	bus := event.NewBus()
	hub := websocket.NewHub(bus)
	go hub.Run()

	directoryService := service.NewDirectoryService(store, bus)
	directoryHandler := handler.NewDirectoryHandler(directoryService)
	fileService := service.NewFileService(store, cfg.AllowedMIMETypes, cfg.ThumbnailRoot, bus)
	fileHandler := handler.NewFileHandler(fileService, cfg.MaxUploadSize)
	trashService, err := service.NewTrashService(store, cfg.TrashRoot, trashRepo)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize trash service: %w", err)
	}
	trashService.SetThumbnailRoot(cfg.ThumbnailRoot)
	auditService := service.NewAuditService(auditRepo)
	auditHandler := handler.NewAuditHandler(auditService)
	docsHandler := handler.NewDocsHandler("./docs/openapi.yaml")
	operationsService := service.NewOperationsService(store, trashService, auditService, bus)
	operationsHandler := handler.NewOperationsHandler(operationsService)
	jobService := service.NewJobService(operationsService, jobRepo, bus)
	jobsHandler := handler.NewJobsHandler(jobService)
	searchService := service.NewSearchService(store, cfg.SearchMaxDepth, cfg.SearchTimeout)
	searchHandler := handler.NewSearchHandler(searchService)
	userHandler := handler.NewUserHandler(authService)
	storageHandler := handler.NewStorageHandler(store, []string{cfg.TrashRoot, cfg.ThumbnailRoot, cfg.ChunkTempDir})
	shareService := service.NewShareService(shareRepo)
	shareHandler := handler.NewShareHandler(shareService, fileService)
	chunkedUploadService, err := service.NewChunkedUploadService(store, cfg.ChunkTempDir, cfg.AllowedMIMETypes, bus)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize chunked upload service: %w", err)
	}
	chunkedUploadHandler := handler.NewChunkedUploadHandler(chunkedUploadService, cfg.ChunkMaxSize)

	appRouter := router.New(cfg, authMiddleware, router.Handlers{
		Auth:          authHandler,
		Directory:     directoryHandler,
		File:          fileHandler,
		Operations:    operationsHandler,
		Search:        searchHandler,
		Audit:         auditHandler,
		Jobs:          jobsHandler,
		Docs:          docsHandler,
		User:          userHandler,
		Storage:       storageHandler,
		Share:         shareHandler,
		ChunkedUpload: chunkedUploadHandler,
	}, hub)

	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	go chunkedUploadService.StartCleanupTicker(cleanupCtx, cfg.ChunkExpiry)

	server := &http.Server{
		Addr:              ":" + cfg.ServerPort,
		Handler:           appRouter,
		ReadHeaderTimeout: cfg.ServerReadHeaderTimeout,
		WriteTimeout:      cfg.ServerWriteTimeout,
		IdleTimeout:       cfg.ServerIdleTimeout,
	}

	return &App{
		server: server,
		db:     db,
		cleanupFuncs: []func(){
			func() {
				db.Close()
			},
			func() {
				cleanupCancel()
			},
		},
	}, nil
}

func (a *App) Run() error {
	go func() {
		slog.Info("server starting", "addr", a.server.Addr)
		if serveErr := a.server.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			slog.Error("server failed", "error", serveErr)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Run cleanup functions
	for _, cleanup := range a.cleanupFuncs {
		cleanup()
	}

	if err := a.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}

	slog.Info("server stopped")
	return nil
}
