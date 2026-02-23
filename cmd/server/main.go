package main

import (
	"log/slog"
	"os"

	"go-file-explorer/internal/app"
	"go-file-explorer/internal/logger"
)

func main() {
	// Initialize custom logger with colors
	logHandler := logger.NewPrettyHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	slog.SetDefault(slog.New(logHandler))

	application, err := app.New()
	if err != nil {
		slog.Error("failed to initialize application", "error", err)
		os.Exit(1)
	}

	if err := application.Run(); err != nil {
		slog.Error("application run failed", "error", err)
		os.Exit(1)
	}
}
