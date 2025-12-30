package handler

import (
	"encoding/base64"
	"encoding/json"
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

type RestockHandler struct {
	repo *repository.RestockRepository
	cfg  *config.Config
}

func NewRestockHandler(repo *repository.RestockRepository, cfg *config.Config) *RestockHandler {
	return &RestockHandler{repo: repo, cfg: cfg}
}

type RestockItemRequest struct {
	ProductID string `json:"productId" binding:"required"`
	Qty       int    `json:"qty" binding:"required"`
}

type CreateRestockRequest struct {
	Longitude float64              `json:"longitude" binding:"required"`
	Latitude  float64              `json:"latitude" binding:"required"`
	Items     []RestockItemRequest `json:"item" binding:"required,dive"`
}

type RestockStatusData struct {
	ID         string  `json:"id"`
	MerchantID string  `json:"merchantId"`
	Status     string  `json:"status"`
	CreatedBy  string  `json:"createdBy"`
	CreatedAt  string  `json:"createdAt"`
	UpdatedBy  string  `json:"updatedBy"`
	UpdatedAt  string  `json:"updatedAt"`
	Longitude  float64 `json:"longitude"`
	Latitude   float64 `json:"latitude"`
}

type RestockStatusResponse struct {
	ResponseCode    string              `json:"responseCode"`
	ResponseMessage string              `json:"responseMessage"`
	Data            []RestockStatusData `json:"data"`
}

type RestockDetailData struct {
	ID          string `json:"id"`
	ProductID   string `json:"productId"`
	ProductName string `json:"productName"`
	Qty         int    `json:"qty"`
}

type RestockDetailResponse struct {
	ResponseCode    string              `json:"responseCode"`
	ResponseMessage string              `json:"responseMessage"`
	Data            []RestockDetailData `json:"data"`
}

// Product struct is already defined in stock.go in the same package

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
		updatedAt := ""
		if r.TsUpdatedAt != nil {
			updatedAt = r.TsUpdatedAt.Format("2006-01-02 15:04:05")
		}

		data[i] = RestockStatusData{
			ID:         r.CID,
			MerchantID: r.CMerchantID,
			Status:     r.CStatus,
			CreatedBy:  r.CCreatedBy,
			CreatedAt:  r.TsCreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedBy:  r.CUpdatedBy,
			UpdatedAt:  updatedAt,
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

// fetchProductDetails fetches product information from the external product API
func (h *RestockHandler) fetchProductDetails(authHeader string) (map[string]Product, error) {
	productURL := h.cfg.ProductServiceURL + "/products"

	slog.Info("Fetching product details from external API", "url", productURL)

	req, err := http.NewRequest("GET", productURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("Failed to fetch products from API", "error", err, "url", productURL)
		return nil, fmt.Errorf("failed to fetch products: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Error("Product API returned non-200 status", "status", resp.StatusCode)
		return nil, fmt.Errorf("product API returned status %d", resp.StatusCode)
	}

	var products []Product
	if err := json.NewDecoder(resp.Body).Decode(&products); err != nil {
		slog.Error("Failed to decode product data", "error", err)
		return nil, fmt.Errorf("failed to decode products: %w", err)
	}

	productMap := make(map[string]Product)
	for _, product := range products {
		productMap[product.ID] = product
	}

	slog.Info("Successfully fetched products", "count", len(products))
	return productMap, nil
}

func (h *RestockHandler) GetRestockDetail(c *gin.Context) {
	restockID := c.Param("id")
	if restockID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"responseCode": "400", "responseMessage": "Restock ID is required"})
		return
	}

	details, err := h.repo.GetRestockDetail(c.Request.Context(), restockID)
	if err != nil {
		slog.Error("Failed to fetch restock details", "error", err, "restockID", restockID)
		c.JSON(http.StatusInternalServerError, gin.H{
			"responseCode":    "500",
			"responseMessage": "Failed to fetch restock details",
		})
		return
	}

	// Fetch product details from external API
	authHeader := c.GetHeader("Authorization")
	productMap, err := h.fetchProductDetails(authHeader)
	if err != nil {
		slog.Warn("Failed to fetch product details, continuing without product names", "error", err)
		productMap = make(map[string]Product)
	}

	data := make([]RestockDetailData, len(details))
	for i, d := range details {
		productName := ""
		if p, found := productMap[d.CProductID]; found {
			productName = p.Name
		} else {
			slog.Warn("Product not found in product service", "product_id", d.CProductID)
		}

		data[i] = RestockDetailData{
			ID:          d.CID,
			ProductID:   d.CProductID,
			ProductName: productName,
			Qty:         d.IQty,
		}
	}

	c.JSON(http.StatusOK, RestockDetailResponse{
		ResponseCode:    "200",
		ResponseMessage: "success",
		Data:            data,
	})
}
