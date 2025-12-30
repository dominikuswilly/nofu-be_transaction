package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nofu-be_transaction/internal/config"
	"nofu-be_transaction/internal/consumer"
	"nofu-be_transaction/internal/handler"
	"nofu-be_transaction/internal/middleware"

	"github.com/gin-gonic/gin"
)

type Server struct {
	router               *gin.Engine
	cfg                  *config.Config
	server               *http.Server
	stockConsumer        *consumer.StockConsumer
	salesConsumer        *consumer.SalesConsumer
	salesProducerHandler *handler.SalesProducerHandler
	defectHandler        *handler.DefectHandler
	restockHandler       *handler.RestockHandler
}

func NewServer(cfg *config.Config, healthHandler *handler.HealthHandler, stockHandler *handler.StockHandler, stockProducerHandler *handler.StockProducerHandler, salesHistoryHandler *handler.SalesHistoryHandler, stockConsumer *consumer.StockConsumer, salesConsumer *consumer.SalesConsumer, salesProducerHandler *handler.SalesProducerHandler, defectHandler *handler.DefectHandler, restockHandler *handler.RestockHandler) *Server {
	router := gin.Default()

	// Add custom logging middleware
	router.Use(middleware.RequestLogger())
	// Set timezone middleware
	router.Use(middleware.TimezoneMiddleware(cfg.DBTimezone))

	// Group routes under /api/transaction
	api := router.Group("/api/transaction")
	{
		// Public/unprotected routes
		api.GET("/health", healthHandler.HealthCheck)

		// Protected routes (require authentication)
		protected := api.Group("")
		protected.Use(middleware.AuthMiddleware(cfg.AuthValidateURL))
		{
			protected.GET("/stock", stockHandler.GetStock)
			protected.POST("/stock/create", stockProducerHandler.CreateStock)
			protected.POST("/sales/create", salesProducerHandler.CreateSales)
			protected.POST("/sales/defect/create", defectHandler.CreateDefect)
			protected.GET("/sales/history", salesHistoryHandler.GetSalesHistory)
			protected.GET("/sales/history/today-grouped", salesHistoryHandler.GetSalesHistoryTodayGrouped)
			protected.POST("/restock/create", restockHandler.CreateRestock)
		}

	}

	return &Server{
		router:               router,
		cfg:                  cfg,
		stockConsumer:        stockConsumer,
		salesConsumer:        salesConsumer,
		salesProducerHandler: salesProducerHandler,
		defectHandler:        defectHandler,
		restockHandler:       restockHandler,
		server: &http.Server{
			Addr:    ":" + cfg.ServerPort,
			Handler: router,
		},
	}
}

func (s *Server) Start() error {
	slog.Info("Starting server", "port", s.cfg.ServerPort)

	// Start RabbitMQ consumers
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start stock consumer
	go func() {
		if err := s.stockConsumer.Start(ctx); err != nil {
			slog.Error("Stock consumer error", "error", err)
		}
	}()

	// Start sales consumer
	go func() {
		if err := s.salesConsumer.Start(ctx); err != nil {
			slog.Error("Sales consumer error", "error", err)
		}
	}()

	// Start HTTP server
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Could not listen on", "addr", s.cfg.ServerPort, "error", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("Shutting down server...")

	// Cancel consumer context
	cancel()

	// Stop consumers
	if err := s.stockConsumer.Stop(); err != nil {
		slog.Error("Error stopping stock consumer", "error", err)
	}
	if err := s.salesConsumer.Stop(); err != nil {
		slog.Error("Error stopping sales consumer", "error", err)
	}

	// Shutdown HTTP server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := s.server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server forced to shutdown: %w", err)
	}

	slog.Info("Server exiting")
	return nil
}
