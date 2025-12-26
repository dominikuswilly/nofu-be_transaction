package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"nofu-be_transaction/internal/config"
	"nofu-be_transaction/internal/models"
	"nofu-be_transaction/pkg/rabbitmq"
)

type SalesProducerHandler struct {
	rabbitClient *rabbitmq.Client
	cfg          *config.Config
}

func NewSalesProducerHandler(rabbitClient *rabbitmq.Client, cfg *config.Config) *SalesProducerHandler {
	return &SalesProducerHandler{
		rabbitClient: rabbitClient,
		cfg:          cfg,
	}
}

// Request models
type SalesDetailRequest struct {
	ProductID string  `json:"productId" binding:"required"`
	Qty       int32   `json:"qty" binding:"required"`
	Price     float64 `json:"price" binding:"required"`
	Currency  string  `json:"currency" binding:"required"`
	StockID   string  `json:"stockId" binding:"required"`
}

type CreateSalesRequest struct {
	SalesDetails []SalesDetailRequest `json:"salesDetails" binding:"required,dive"`
}

func (h *SalesProducerHandler) CreateSales(c *gin.Context) {
	var req CreateSalesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("Invalid request body", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Extract claims from context - try multiple common keys
	var userID string
	keys := []string{"sub", "id", "user_id", "userId"}
	for _, key := range keys {
		if val := c.GetString(key); val != "" {
			userID = val
			break
		}
	}

	if userID == "" {
		slog.Error("User ID not found in context (checked: sub, id, user_id, userId)")
		// Log all keys in context for debugging (be careful with sensitive data)
		// for k, v := range c.Keys {
		// 	slog.Info("Context key", "key", k, "value", v)
		// }
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User ID not found in token"})
		return
	}

	// Try to finding specific merchant_id claim, or fallback to userID
	merchantID := c.GetString("merchant_id")
	if merchantID == "" {
		merchantID = c.GetString("merchantId")
	}
	if merchantID == "" {
		merchantID = userID
	}

	// Construct message
	salesDetails := make([]models.SalesDetailMessage, len(req.SalesDetails))
	for i, d := range req.SalesDetails {
		salesDetails[i] = models.SalesDetailMessage{
			ProductID: d.ProductID,
			Qty:       d.Qty,
			Price:     d.Price,
			Currency:  d.Currency,
			StockID:   d.StockID,
		}
	}

	msg := models.SalesMessage{
		UserID:       userID,
		MerchantID:   merchantID,
		SalesDetails: salesDetails,
	}

	body, err := json.Marshal(msg)
	if err != nil {
		slog.Error("Failed to marshal message", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	// Publish to RabbitMQ
	err = h.rabbitClient.Publish(h.cfg.SalesExchange, h.cfg.SalesRoutingKey, body)
	if err != nil {
		slog.Error("Failed to publish message", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to queue sales creation"})
		return
	}

	slog.Info("Sales creation message published",
		"user_id", userID,
		"merchant_id", merchantID,
		"details_count", len(salesDetails))

	c.JSON(http.StatusAccepted, gin.H{
		"message": "Sales creation request accepted",
		"status":  "pending",
	})
}
