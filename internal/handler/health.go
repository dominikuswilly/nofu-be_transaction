package handler

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type HealthHandler struct {
	DB *pgxpool.Pool
}

func NewHealthHandler(db *pgxpool.Pool) *HealthHandler {
	return &HealthHandler{DB: db}
}

func (h *HealthHandler) HealthCheck(c *gin.Context) {
	ctx := c.Request.Context()

	if err := h.DB.Ping(ctx); err != nil {
		slog.Error("Health check failed", "error", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "unavailable",
			"error":  "database connection failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
	})
}
