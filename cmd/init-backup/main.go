package main

import (
	"log/slog"
	"os"

	initbackup "github.com/GreedyKomodoDragon/redis-operator/internal/init-backup"
)

func main() {
	// Initialize structured logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Create S3 object store
	objectStore, err := initbackup.NewS3Store(logger)
	if err != nil {
		logger.Error("Failed to create S3 store", "error", err)
		os.Exit(1)
	}
	defer objectStore.Close()

	// Create and run the service
	service := initbackup.NewService(objectStore)
	if err := service.Run(); err != nil {
		logger.Error("Service failed", "error", err)
		os.Exit(1)
	}
}
