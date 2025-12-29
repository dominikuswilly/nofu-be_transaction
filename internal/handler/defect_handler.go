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

type DefectHandler struct {
	stockRepo *repository.StockRepository
	cfg       *config.Config
}

func NewDefectHandler(stockRepo *repository.StockRepository, cfg *config.Config) *DefectHandler {
	return &DefectHandler{
		stockRepo: stockRepo,
		cfg:       cfg,
	}
}

// Request models
type DefectDetailRequest struct {
	ProductID     string  `json:"productId" binding:"required"`
	Qty           int32   `json:"qty" binding:"required"`
	Price         float64 `json:"price" binding:"required"`
	Currency      string  `json:"currency" binding:"required"`
	StockDetailID string  `json:"stockDetailId" binding:"required"`
}

type CreateDefectRequest struct {
	DefectDetails []DefectDetailRequest `json:"defectDetails" binding:"required,dive"`
}

func (h *DefectHandler) CreateDefect(c *gin.Context) {
	var req CreateDefectRequest
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

	// Parse JWT manually
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

	// Get merchant_id from claims or context
	merchantID := c.GetString("merchant_id")
	if merchantID == "" {
		merchantID = c.GetString("merchantId")
	}
	if merchantID == "" {
		// Fallback to claims if context is empty
		if mID, ok := claims["merchant_id"].(string); ok {
			merchantID = mID
		} else if mID, ok := claims["merchantId"].(string); ok {
			merchantID = mID
		}
	}
	if merchantID == "" {
		merchantID = userID
	}

	// Generate UUIDs
	defectID, err := uuid.NewV7()
	if err != nil {
		slog.Error("Failed to generate defect UUID", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	// Create SalesDefectMaster
	defectMaster := &models.SalesDefectMaster{
		CID:         defectID.String(),
		CCreatedBy:  userID,
		TsCreatedAt: time.Now(),
		CMerchantID: merchantID,
	}

	// Create SalesDefectDetails
	defectDetails := make([]models.SalesDefectDetail, len(req.DefectDetails))
	for i, d := range req.DefectDetails {
		detailID, err := uuid.NewV7()
		if err != nil {
			slog.Error("Failed to generate defect detail UUID", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			return
		}

		defectDetails[i] = models.SalesDefectDetail{
			CID:            detailID.String(),
			CSalesDefectID: defectMaster.CID,
			CProductID:     d.ProductID,
			IQty:           d.Qty,
			DPrice:         d.Price,
			CCurrency:      d.Currency,
			CStockDetailID: d.StockDetailID,
		}
	}

	// Process defect transaction
	err = h.stockRepo.ProcessDefectTransaction(c.Request.Context(), defectMaster, defectDetails)
	if err != nil {
		slog.Error("Failed to process defect transaction", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create defect record"})
		return
	}

	slog.Info("Defect record created successfully",
		"defect_id", defectMaster.CID,
		"merchant_id", merchantID,
		"details_count", len(defectDetails))

	c.JSON(http.StatusCreated, gin.H{
		"message": "Defect record created successfully",
		"status":  "success",
		"data": gin.H{
			"defectId":   defectMaster.CID,
			"merchantId": merchantID,
		},
	})
}
