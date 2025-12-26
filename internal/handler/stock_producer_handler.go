package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"nofu-be_transaction/internal/config"
	"nofu-be_transaction/internal/models"
	"nofu-be_transaction/pkg/rabbitmq"
)

type StockProducerHandler struct {
	RabbitClient *rabbitmq.Client
	Config       *config.Config
}

func NewStockProducerHandler(rabbitClient *rabbitmq.Client, cfg *config.Config) *StockProducerHandler {
	return &StockProducerHandler{
		RabbitClient: rabbitClient,
		Config:       cfg,
	}
}

// CreateStock handles POST /api/transaction/stock/create
func (h *StockProducerHandler) CreateStock(c *gin.Context) {
	var stockMsg models.StockMessage

	// Parse request body
	if err := c.ShouldBindJSON(&stockMsg); err != nil {
		slog.Error("Failed to parse request body", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"responseCode":    "400",
			"responseMessage": "Invalid request body",
			"error":           err.Error(),
		})
		return
	}

	// Validate request
	if err := h.validateStockMessage(&stockMsg); err != nil {
		slog.Error("Invalid stock message", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"responseCode":    "400",
			"responseMessage": "Validation failed",
			"error":           err.Error(),
		})
		return
	}

	slog.Info("Received stock creation request",
		"user_id", stockMsg.UserID,
		"merchant_id", stockMsg.MerchantID,
		"detail_count", len(stockMsg.StockDetails))

	// Generate routing key: stock.merchant.{merchantId}.create
	routingKey := fmt.Sprintf("stock.merchant.%s.create", stockMsg.MerchantID)

	// Marshal message to JSON
	messageBody, err := json.Marshal(stockMsg)
	if err != nil {
		slog.Error("Failed to marshal message", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"responseCode":    "500",
			"responseMessage": "Failed to process message",
		})
		return
	}

	// Publish message to RabbitMQ
	err = h.RabbitClient.Publish(h.Config.RabbitMQExchange, routingKey, messageBody)
	if err != nil {
		slog.Error("Failed to publish message to RabbitMQ", "error", err, "routing_key", routingKey)
		c.JSON(http.StatusInternalServerError, gin.H{
			"responseCode":    "500",
			"responseMessage": "Failed to publish message to queue",
		})
		return
	}

	slog.Info("Stock message published successfully",
		"routing_key", routingKey,
		"merchant_id", stockMsg.MerchantID,
		"detail_count", len(stockMsg.StockDetails))

	c.JSON(http.StatusOK, gin.H{
		"responseCode":    "200",
		"responseMessage": "Stock message published successfully",
		"data": gin.H{
			"routingKey":  routingKey,
			"exchange":    h.Config.RabbitMQExchange,
			"merchantId":  stockMsg.MerchantID,
			"detailCount": len(stockMsg.StockDetails),
		},
	})
}

// validateStockMessage validates the stock message
func (h *StockProducerHandler) validateStockMessage(msg *models.StockMessage) error {
	if msg.UserID == "" {
		return fmt.Errorf("userId is required")
	}
	if msg.MerchantID == "" {
		return fmt.Errorf("merchantId is required")
	}
	if len(msg.StockDetails) == 0 {
		return fmt.Errorf("stockDetails cannot be empty")
	}

	for i, detail := range msg.StockDetails {
		if detail.ProductID == "" {
			return fmt.Errorf("productId is required for detail at index %d", i)
		}
		if detail.Price <= 0 {
			return fmt.Errorf("price must be positive for detail at index %d", i)
		}
		if detail.Qty <= 0 {
			return fmt.Errorf("qty must be positive for detail at index %d", i)
		}
		if detail.Currency == "" {
			return fmt.Errorf("currency is required for detail at index %d", i)
		}
	}

	return nil
}
