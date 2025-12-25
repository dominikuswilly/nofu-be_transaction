package main

import (
	"context"
	"log/slog"
	"os"

	"nofu-be_transaction/internal/config"
	"nofu-be_transaction/internal/handler"
	"nofu-be_transaction/internal/server"
	"nofu-be_transaction/pkg/logger"
	"nofu-be_transaction/pkg/postgres"
)

func main() {
	// Initialize logger
	logger.InitLogger()

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	// Connect to database
	// Create a context that can be cancelled
	ctx := context.Background()
	db, err := postgres.NewClient(ctx, cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName)
	if err != nil {
		slog.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	slog.Info("Connected to database successfully", "timezone", cfg.DBTimezone)

	// Initialize handlers
	healthHandler := handler.NewHealthHandler(db)
	stockHandler := handler.NewStockHandler(db)

	// Initialize server
	srv := server.NewServer(cfg, healthHandler, stockHandler)

	// Start server
	if err := srv.Start(); err != nil {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
