package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"nofu-be_transaction/internal/config"
	"nofu-be_transaction/internal/models"
	"nofu-be_transaction/internal/repository"
)

type StockProducerHandler struct {
	StockRepo *repository.StockRepository
	Config    *config.Config
}

func NewStockProducerHandler(stockRepo *repository.StockRepository, cfg *config.Config) *StockProducerHandler {
	return &StockProducerHandler{
		StockRepo: stockRepo,
		Config:    cfg,
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

	// Prepare StockMaster
	stockID, err := uuid.NewV7()
	if err != nil {
		slog.Error("Failed to generate stock UUID", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"responseCode":    "500",
			"responseMessage": "Internal server error",
		})
		return
	}

	status := stockMsg.Status
	if status == "" {
		status = "waiting approval from merchant"
	}

	stockMaster := &models.StockMaster{
		CID:         stockID.String(),
		CCreatedBy:  stockMsg.UserID, // Using UserID for created_by
		CMerchantID: stockMsg.MerchantID,
		CAdminID:    stockMsg.UserID, // Using UserID for admin_id for now, adjust if needed
		CStatus:     status,
		TsCreatedAt: time.Now(),
	}

	// Prepare StockDetails
	var stockDetails []models.StockDetail
	for _, detailMsg := range stockMsg.StockDetails {
		detailID, err := uuid.NewV7()
		if err != nil {
			slog.Error("Failed to generate detail UUID", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"responseCode":    "500",
				"responseMessage": "Internal server error",
			})
			return
		}

		stockDetails = append(stockDetails, models.StockDetail{
			CID:         detailID.String(),
			CStockID:    stockMaster.CID,
			CProductID:  detailMsg.ProductID,
			DPrice:      detailMsg.Price,
			IQty:        detailMsg.Qty,
			IQtyCurrent: detailMsg.Qty,
			IQtyRestock: 0,
			CCurrency:   detailMsg.Currency,
		})
	}

	// Save to database using transaction
	err = h.StockRepo.InsertStockTransaction(c.Request.Context(), stockMaster, stockDetails)
	if err != nil {
		slog.Error("Failed to save stock transaction", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"responseCode":    "500",
			"responseMessage": "Failed to save stock data",
			"error":           err.Error(),
		})
		return
	}

	slog.Info("Stock transaction saved successfully",
		"stock_id", stockMaster.CID,
		"merchant_id", stockMsg.MerchantID,
		"detail_count", len(stockDetails))

	c.JSON(http.StatusOK, gin.H{
		"responseCode":    "200",
		"responseMessage": "Stock created successfully",
		"data": gin.H{
			"id":          stockMaster.ID,
			"stockId":     stockMaster.CID,
			"merchantId":  stockMsg.MerchantID,
			"detailCount": len(stockDetails),
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

// UpdateStockStatus handles PATCH /api/transaction/stock/:stock_master_id?action=approve|reject
func (h *StockProducerHandler) UpdateStockStatus(c *gin.Context) {
	stockID := c.Param("stock_master_id")
	action := c.Query("action")

	// Get userId from claims (set by AuthMiddleware)
	userID, exists := c.Get("userId")
	if !exists {
		slog.Error("userId not found in context")
		c.JSON(http.StatusUnauthorized, gin.H{
			"responseCode":    "401",
			"responseMessage": "Unauthorized",
		})
		return
	}

	var status string
	switch action {
	case "approve":
		status = "approved by merchant"
	case "reject":
		status = "rejected by merchant"
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"responseCode":    "400",
			"responseMessage": "Invalid action. Use 'approve' or 'reject'",
		})
		return
	}

	err := h.StockRepo.UpdateStockStatus(c.Request.Context(), stockID, status, userID.(string))
	if err != nil {
		slog.Error("Failed to update stock status", "error", err, "stock_id", stockID)
		if strings.Contains(err.Error(), "no stock master found") {
			c.JSON(http.StatusNotFound, gin.H{
				"responseCode":    "404",
				"responseMessage": "Stock not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"responseCode":    "500",
			"responseMessage": "Internal server error",
			"error":           err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"responseCode":    "200",
		"responseMessage": fmt.Sprintf("Stock %sed successfully", action),
	})
}
