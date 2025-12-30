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

	"nofu-be_transaction/internal/models"
	"nofu-be_transaction/internal/repository"
)

type RestockHandler struct {
	repo *repository.RestockRepository
}

func NewRestockHandler(repo *repository.RestockRepository) *RestockHandler {
	return &RestockHandler{repo: repo}
}

type RestockItemRequest struct {
	ProductID string `json:"productId" binding:"required"`
	Qty       int    `json:"qty" binding:"required"`
}

type CreateRestockRequest struct {
	Longitude string               `json:"longitude"`
	Latitude  string               `json:"latitude"`
	Items     []RestockItemRequest `json:"item" binding:"required,dive"`
}

type RestockStatusData struct {
	ID         string `json:"id"`
	MerchantID string `json:"merchantId"`
	Status     string `json:"status"`
	CreatedBy  string `json:"createdBy"`
	CreatedAt  string `json:"createdAt"`
	UpdatedBy  string `json:"updatedBy"`
	UpdatedAt  string `json:"updatedAt"`
	Longitude  string `json:"longitude"`
	Latitude   string `json:"latitude"`
}

type RestockStatusResponse struct {
	ResponseCode    string              `json:"responseCode"`
	ResponseMessage string              `json:"responseMessage"`
	Data            []RestockStatusData `json:"data"`
}

func (h *RestockHandler) CreateRestock(c *gin.Context) {
	var req CreateRestockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("Invalid request body", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"responseCode":    "400",
			"responseMessage": "Invalid request body: " + err.Error(),
		})
		return
	}

	// Extract claims from Authorization header (logic borrowed from SalesProducerHandler)
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"responseCode": "401", "responseMessage": "Missing Authorization header"})
		return
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		c.JSON(http.StatusUnauthorized, gin.H{"responseCode": "401", "responseMessage": "Invalid token format"})
		return
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"responseCode": "401", "responseMessage": "Invalid token payload"})
		return
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"responseCode": "401", "responseMessage": "Invalid token claims"})
		return
	}

	userID, _ := claims["sub"].(string)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"responseCode": "401", "responseMessage": "User ID (sub) not found in token"})
		return
	}

	merchantID := userID // As per requirement: c_merchant_id : by authorization token (claims.sub)

	// Prepare data
	masterID, _ := uuid.NewV7()
	now := time.Now()
	master := &models.StockRestockMaster{
		CID:         masterID.String(),
		CMerchantID: merchantID,
		CStatus:     "PENDING",
		CCreatedBy:  userID,
		TsCreatedAt: now,
		DLongitude:  req.Longitude,
		DLatitude:   req.Latitude,
	}

	details := make([]models.StockRestockDetail, len(req.Items))
	for i, item := range req.Items {
		detailID, _ := uuid.NewV7()
		details[i] = models.StockRestockDetail{
			CID:             detailID.String(),
			CStockRestockID: master.CID,
			CProductID:      item.ProductID,
			IQty:            item.Qty,
			CCreatedBy:      userID,
		}
	}

	historyID, _ := uuid.NewV7()
	history := &models.StockRestockHistory{
		CID:             historyID.String(),
		CStockRestockID: master.CID,
		ISeq:            1,
		CStatus:         "PENDING", // initiate "PENDING"
		CCreatedBy:      userID,
		TsCreatedAt:     time.Now(),
	}

	if err := h.repo.CreateRestock(c.Request.Context(), master, details, history); err != nil {
		slog.Error("Failed to create restock", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"responseCode":    "500",
			"responseMessage": "Failed to create restock",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"responseCode":    "201",
		"responseMessage": "Success",
		"data": gin.H{
			"requestID": master.CID,
		},
	})
}

func (h *RestockHandler) GetRestock(c *gin.Context) {
	// Extract claims from Authorization header
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"responseCode": "401", "responseMessage": "Missing Authorization header"})
		return
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		c.JSON(http.StatusUnauthorized, gin.H{"responseCode": "401", "responseMessage": "Invalid token format"})
		return
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"responseCode": "401", "responseMessage": "Invalid token payload"})
		return
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"responseCode": "401", "responseMessage": "Invalid token claims"})
		return
	}

	userID, _ := claims["sub"].(string)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"responseCode": "401", "responseMessage": "User ID (sub) not found in token"})
		return
	}

	merchantID := userID // As per pattern in CreateRestock

	restocks, err := h.repo.GetRestockByMerchantID(c.Request.Context(), merchantID)
	if err != nil {
		slog.Error("Failed to fetch restock status", "error", err, "merchantID", merchantID)
		c.JSON(http.StatusInternalServerError, gin.H{
			"responseCode":    "500",
			"responseMessage": "Failed to fetch restock status",
		})
		return
	}

	data := make([]RestockStatusData, len(restocks))
	for i, r := range restocks {
		data[i] = RestockStatusData{
			ID:         r.CID,
			MerchantID: r.CMerchantID,
			Status:     r.CStatus,
			CreatedBy:  r.CCreatedBy,
			CreatedAt:  r.TsCreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedBy:  r.CUpdatedBy,
			UpdatedAt:  r.TsUpdatedAt.Format("2006-01-02 15:04:05"),
			Longitude:  r.DLongitude,
			Latitude:   r.DLatitude,
		}
	}

	c.JSON(http.StatusOK, RestockStatusResponse{
		ResponseCode:    "200",
		ResponseMessage: "success",
		Data:            data,
	})
}
