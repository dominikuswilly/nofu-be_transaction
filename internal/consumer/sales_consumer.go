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

type SalesConsumer struct {
	rabbitClient *rabbitmq.Client
	stockRepo    *repository.StockRepository
	queueName    string
	exchangeName string
	routingKey   string
}

func NewSalesConsumer(rabbitClient *rabbitmq.Client, stockRepo *repository.StockRepository, queueName, exchangeName, routingKey string) *SalesConsumer {
	return &SalesConsumer{
		rabbitClient: rabbitClient,
		stockRepo:    stockRepo,
		queueName:    queueName,
		exchangeName: exchangeName,
		routingKey:   routingKey,
	}
}

// Start begins consuming sales messages from RabbitMQ
func (c *SalesConsumer) Start(ctx context.Context) error {
	slog.Info("Starting sales consumer", "queue", c.queueName, "exchange", c.exchangeName, "routing_key", c.routingKey)

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

// handleMessage processes incoming sales messages
func (c *SalesConsumer) handleMessage(body []byte) error {
	slog.Info("Processing sales message", "body_size", len(body))

	// Parse message
	var msg models.SalesMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		slog.Error("Failed to unmarshal message", "error", err, "body", string(body))
		return fmt.Errorf("failed to unmarshal message: %w", err)
	}

	// Validate message
	if err := c.validateMessage(&msg); err != nil {
		slog.Error("Invalid message", "error", err)
		return err
	}

	slog.Info("Parsed sales message",
		"user_id", msg.UserID,
		"merchant_id", msg.MerchantID,
		"detail_count", len(msg.SalesDetails))

	// Generate UUID v7 for sales master
	salesMasterID, err := uuid.NewV7()
	if err != nil {
		slog.Error("Failed to generate UUID v7", "error", err)
		return fmt.Errorf("failed to generate UUID v7: %w", err)
	}

	// Create sales master
	salesMaster := &models.SalesMaster{
		CID:            salesMasterID.String(),
		TsCreatedAt:    time.Now(),
		CCreatedBy:     msg.UserID,
		CMerchantID:    msg.MerchantID,
		CPaymentMethod: msg.PaymentMethod,
		DTotalPayment:  msg.TotalPayment,
	}

	// Create sales details
	salesDetails := make([]models.SalesDetail, 0, len(msg.SalesDetails))
	for _, detail := range msg.SalesDetails {
		detailID, err := uuid.NewV7()
		if err != nil {
			slog.Error("Failed to generate UUID v7 for detail", "error", err)
			return fmt.Errorf("failed to generate UUID v7 for detail: %w", err)
		}

		salesDetails = append(salesDetails, models.SalesDetail{
			CID:            detailID.String(),
			CSalesID:       salesMasterID.String(),
			CProductID:     detail.ProductID,
			IQty:           detail.Qty,
			DPrice:         detail.Price,
			CCurrency:      detail.Currency,
			CStockDetailID: detail.StockDetailID,
		})
	}

	// Process sales transaction (reduce stock and insert sales records)
	ctx := context.Background()
	if err := c.stockRepo.ProcessSalesTransaction(ctx, salesMaster, salesDetails); err != nil {
		slog.Error("Failed to process sales transaction", "error", err)
		return fmt.Errorf("failed to process sales transaction: %w", err)
	}

	slog.Info("Sales message processed successfully",
		"sales_id", salesMaster.CID,
		"merchant_id", msg.MerchantID,
		"detail_count", len(salesDetails))

	return nil
}

// validateMessage validates the sales message
func (c *SalesConsumer) validateMessage(msg *models.SalesMessage) error {
	if msg.UserID == "" {
		return fmt.Errorf("userId is required")
	}
	if msg.MerchantID == "" {
		return fmt.Errorf("merchantId is required")
	}
	if msg.PaymentMethod == "" {
		return fmt.Errorf("paymentMethod is required")
	}
	if msg.TotalPayment <= 0 {
		return fmt.Errorf("totalPayment must be positive")
	}
	if len(msg.SalesDetails) == 0 {
		return fmt.Errorf("salesDetails cannot be empty")
	}

	for i, detail := range msg.SalesDetails {
		if detail.ProductID == "" {
			return fmt.Errorf("productId is required for detail at index %d", i)
		}
		if detail.Qty <= 0 {
			return fmt.Errorf("qty must be positive for detail at index %d", i)
		}
		if detail.Price <= 0 {
			return fmt.Errorf("price must be positive for detail at index %d", i)
		}
		if detail.Currency == "" {
			return fmt.Errorf("currency is required for detail at index %d", i)
		}
		if detail.StockDetailID == "" {
			return fmt.Errorf("stockDetailId is required for detail at index %d", i)
		}
	}

	return nil
}

// Stop gracefully shuts down the consumer
func (c *SalesConsumer) Stop() error {
	slog.Info("Stopping sales consumer")
	return c.rabbitClient.Close()
}
