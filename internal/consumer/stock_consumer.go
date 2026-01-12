package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"nofu-be_transaction/internal/models"
	"nofu-be_transaction/internal/repository"
	"nofu-be_transaction/pkg/rabbitmq"
)

type StockConsumer struct {
	rabbitClient *rabbitmq.Client
	stockRepo    *repository.StockRepository
	queueName    string
	exchangeName string
	routingKey   string
}

func NewStockConsumer(rabbitClient *rabbitmq.Client, stockRepo *repository.StockRepository, queueName, exchangeName, routingKey string) *StockConsumer {
	return &StockConsumer{
		rabbitClient: rabbitClient,
		stockRepo:    stockRepo,
		queueName:    queueName,
		exchangeName: exchangeName,
		routingKey:   routingKey,
	}
}

// Start begins consuming messages from RabbitMQ
func (c *StockConsumer) Start(ctx context.Context) error {
	slog.Info("Starting stock consumer", "queue", c.queueName, "exchange", c.exchangeName, "routing_key", c.routingKey)

	// Declare the topic exchange
	if err := c.rabbitClient.DeclareExchange(c.exchangeName, "topic"); err != nil {
		return fmt.Errorf("failed to declare exchange: %w", err)
	}

	// Declare the queue to ensure it exists
	if err := c.rabbitClient.DeclareQueue(c.queueName); err != nil {
		return fmt.Errorf("failed to declare queue: %w", err)
	}

	// Bind the queue to the exchange with routing key pattern
	if err := c.rabbitClient.BindQueue(c.queueName, c.exchangeName, c.routingKey); err != nil {
		return fmt.Errorf("failed to bind queue to exchange: %w", err)
	}

	// Start consuming messages
	return c.rabbitClient.Consume(ctx, c.queueName, c.handleMessage)
}

// handleMessage processes incoming stock messages
func (c *StockConsumer) handleMessage(body []byte) error {
	slog.Info("Processing stock message", "body_size", len(body))

	// Parse message
	var msg models.StockMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		slog.Error("Failed to unmarshal message", "error", err, "body", string(body))
		return fmt.Errorf("failed to unmarshal message: %w", err)
	}

	// Validate message
	if err := c.validateMessage(&msg); err != nil {
		slog.Error("Invalid message", "error", err)
		return err
	}

	slog.Info("Parsed stock message",
		"user_id", msg.UserID,
		"merchant_id", msg.MerchantID,
		"detail_count", len(msg.StockDetails))

	// Generate UUID v7 for stock master
	stockMasterID, err := uuid.NewV7()
	if err != nil {
		slog.Error("Failed to generate UUID v7", "error", err)
		return fmt.Errorf("failed to generate UUID v7: %w", err)
	}

	// Create stock master
	stockMaster := &models.StockMaster{
		CID:         stockMasterID.String(),
		CCreatedBy:  msg.UserID,
		CMerchantID: msg.MerchantID,
		CAdminID:    msg.UserID,
		TsCreatedAt: time.Now(),
	}

	// Create stock details
	stockDetails := make([]models.StockDetail, 0, len(msg.StockDetails))
	for _, detail := range msg.StockDetails {
		detailID, err := uuid.NewV7()
		if err != nil {
			slog.Error("Failed to generate UUID v7 for detail", "error", err)
			return fmt.Errorf("failed to generate UUID v7 for detail: %w", err)
		}

		stockDetails = append(stockDetails, models.StockDetail{
			CID:        detailID.String(),
			CStockID:   stockMasterID.String(),
			CProductID: detail.ProductID,
			DPrice:     detail.Price,
			IQty:       detail.Qty,
			CCurrency:  detail.Currency,
		})
	}

	// Insert into database using transaction
	ctx := context.Background()
	if err := c.stockRepo.InsertStockTransaction(ctx, stockMaster, stockDetails, string(body)); err != nil {
		slog.Error("Failed to insert stock transaction", "error", err)
		return fmt.Errorf("failed to insert stock transaction: %w", err)
	}

	slog.Info("Stock message processed successfully",
		"stock_id", stockMaster.CID,
		"merchant_id", msg.MerchantID,
		"detail_count", len(stockDetails))

	return nil
}

// validateMessage validates the stock message
func (c *StockConsumer) validateMessage(msg *models.StockMessage) error {
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

// Stop gracefully shuts down the consumer
func (c *StockConsumer) Stop() error {
	slog.Info("Stopping stock consumer")
	return c.rabbitClient.Close()
}
