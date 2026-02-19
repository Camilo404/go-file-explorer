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
	"go-file-explorer/internal/handler"
	"go-file-explorer/internal/middleware"
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
	authService, err := service.NewAuthService(cfg.UsersFile, cfg.JWTSecret, cfg.JWTAccessTTL, cfg.JWTRefreshTTL)
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
	trashService, err := service.NewTrashService(store, cfg.TrashRoot, cfg.TrashIndexFile)
	if err != nil {
		slog.Error("failed to initialize trash service", "error", err)
		os.Exit(1)
	}
	auditService, err := service.NewAuditService(cfg.AuditLogFile)
	if err != nil {
		slog.Error("failed to initialize audit service", "error", err)
		os.Exit(1)
	}
	operationsService := service.NewOperationsService(store, trashService, auditService)
	operationsHandler := handler.NewOperationsHandler(operationsService)
	searchService := service.NewSearchService(store, cfg.SearchMaxDepth, cfg.SearchTimeout)
	searchHandler := handler.NewSearchHandler(searchService)
	appRouter := router.New(cfg, authMiddleware, authHandler, directoryHandler, fileHandler, operationsHandler, searchHandler)

	server := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      appRouter,
		ReadTimeout:  cfg.ServerReadTimeout,
		WriteTimeout: cfg.ServerWriteTimeout,
		IdleTimeout:  cfg.ServerIdleTimeout,
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

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}

	slog.Info("server stopped")
}
