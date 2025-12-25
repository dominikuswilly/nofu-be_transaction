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
	"nofu-be_transaction/internal/handler"
	"nofu-be_transaction/internal/middleware"

	"github.com/gin-gonic/gin"
)

type Server struct {
	router *gin.Engine
	cfg    *config.Config
	server *http.Server
}

func NewServer(cfg *config.Config, healthHandler *handler.HealthHandler, stockHandler *handler.StockHandler) *Server {
	router := gin.Default()

	// Add custom logging middleware
	router.Use(middleware.RequestLogger())

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
		}
	}

	return &Server{
		router: router,
		cfg:    cfg,
		server: &http.Server{
			Addr:    ":" + cfg.ServerPort,
			Handler: router,
		},
	}
}

func (s *Server) Start() error {
	slog.Info("Starting server", "port", s.cfg.ServerPort)

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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("server forced to shutdown: %w", err)
	}

	slog.Info("Server exiting")
	return nil
}
