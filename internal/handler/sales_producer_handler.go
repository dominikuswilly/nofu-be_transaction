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

	// Extract claims from context
	userID := c.GetString("sub")
	if userID == "" {
		slog.Error("User ID not found in context")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User ID not found in token"})
		return
	}

	// Assuming merchantId is also based on sub (as requested)
	// Or check if there is a specific merchant_id claim.
	// The user said: "userId and merchantId based on claims.sub"
	merchantID := userID

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
