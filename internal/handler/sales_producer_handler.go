package handler

import (
	"encoding/base64"
	"encoding/json"
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

type SalesProducerHandler struct {
	stockRepo *repository.StockRepository
	cfg       *config.Config
}

func NewSalesProducerHandler(stockRepo *repository.StockRepository, cfg *config.Config) *SalesProducerHandler {
	return &SalesProducerHandler{
		stockRepo: stockRepo,
		cfg:       cfg,
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

	// Extract claims from Authorization header
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		slog.Error("Missing Authorization header")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header is required"})
		return
	}

	// Remove "Bearer " prefix
	tokenString := ""
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		tokenString = authHeader[7:]
	} else {
		tokenString = authHeader
	}

	// Parse JWT manually (since we don't have a library and just need the payload)
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		slog.Error("Invalid token format")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token format"})
		return
	}

	// Decode payload (2nd part)
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		slog.Error("Failed to decode token payload", "error", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token payload"})
		return
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		slog.Error("Failed to unmarshal token payload", "error", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims"})
		return
	}

	// Get sub from claims
	var userID string
	if sub, ok := claims["sub"].(string); ok {
		userID = sub
	}

	if userID == "" {
		slog.Error("User ID (sub) not found in token claims")
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

	// Generate UUIDs
	salesID, err := uuid.NewV7()
	if err != nil {
		slog.Error("Failed to generate sales UUID", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	// Create SalesMaster
	salesMaster := &models.SalesMaster{
		CID:         salesID.String(),
		CCreatedBy:  userID,
		CMerchantID: merchantID,
		TsCreatedAt: time.Now(),
	}

	// Create SalesDetails
	salesDetails := make([]models.SalesDetail, len(req.SalesDetails))
	for i, d := range req.SalesDetails {
		detailID, err := uuid.NewV7()
		if err != nil {
			slog.Error("Failed to generate sales detail UUID", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			return
		}

		salesDetails[i] = models.SalesDetail{
			CID:        detailID.String(),
			CSalesID:   salesMaster.CID,
			CProductID: d.ProductID,
			IQty:       d.Qty,
			DPrice:     d.Price,
			CCurrency:  d.Currency,
			CStockID:   d.StockID,
		}
	}

	// Process sales transaction
	err = h.stockRepo.ProcessSalesTransaction(c.Request.Context(), salesMaster, salesDetails)
	if err != nil {
		slog.Error("Failed to process sales transaction", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create sales"})
		return
	}

	slog.Info("Sales created successfully",
		"sales_id", salesMaster.CID,
		"merchant_id", merchantID,
		"details_count", len(salesDetails))

	c.JSON(http.StatusCreated, gin.H{
		"message": "Sales created successfully",
		"status":  "success",
		"data": gin.H{
			"salesId":    salesMaster.CID,
			"merchantId": merchantID,
		},
	})
}
