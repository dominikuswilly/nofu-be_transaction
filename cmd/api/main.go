package main

import (
	"context"
	"log/slog"
	"os"

	"nofu-be_transaction/internal/config"
	"nofu-be_transaction/internal/consumer"
	"nofu-be_transaction/internal/handler"
	"nofu-be_transaction/internal/repository"
	"nofu-be_transaction/internal/server"
	"nofu-be_transaction/pkg/logger"
	"nofu-be_transaction/pkg/postgres"
	"nofu-be_transaction/pkg/rabbitmq"
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
	db, err := postgres.NewClient(ctx, cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBTimezone)
	if err != nil {
		slog.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	slog.Info("Connected to database successfully", "timezone", cfg.DBTimezone)

	// Connect to RabbitMQ - Create separate clients for each consumer
	rabbitClientStock, err := rabbitmq.NewClient(cfg.RabbitMQURL)
	if err != nil {
		slog.Error("Failed to connect to RabbitMQ for stock consumer", "error", err)
		os.Exit(1)
	}
	defer rabbitClientStock.Close()

	rabbitClientSales, err := rabbitmq.NewClient(cfg.RabbitMQURL)
	if err != nil {
		slog.Error("Failed to connect to RabbitMQ for sales consumer", "error", err)
		os.Exit(1)
	}
	defer rabbitClientSales.Close()

	// Keep one general client for producers/handlers
	rabbitClient, err := rabbitmq.NewClient(cfg.RabbitMQURL)
	if err != nil {
		slog.Error("Failed to connect to RabbitMQ", "error", err)
		os.Exit(1)
	}
	defer rabbitClient.Close()

	slog.Info("Connected to RabbitMQ successfully")

	// Initialize repositories
	stockRepo := repository.NewStockRepository(db)

	// Initialize consumers with their own dedicated RabbitMQ clients
	stockConsumer := consumer.NewStockConsumer(rabbitClientStock, stockRepo, cfg.RabbitMQQueue, cfg.RabbitMQExchange, cfg.RabbitMQRoutingKey)
	salesConsumer := consumer.NewSalesConsumer(rabbitClientSales, stockRepo, cfg.SalesQueue, cfg.SalesExchange, cfg.SalesRoutingKey)

	// Initialize handlers
	healthHandler := handler.NewHealthHandler(db)
	stockHandler := handler.NewStockHandler(db, cfg)
	stockProducerHandler := handler.NewStockProducerHandler(stockRepo, cfg)
	salesProducerHandler := handler.NewSalesProducerHandler(rabbitClient, cfg)

	// Initialize server
	srv := server.NewServer(cfg, healthHandler, stockHandler, stockProducerHandler, stockConsumer, salesConsumer, salesProducerHandler)

	// Start server
	if err := srv.Start(); err != nil {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
