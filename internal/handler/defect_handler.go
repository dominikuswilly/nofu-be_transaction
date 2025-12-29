package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
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

// Response models
type SalesDefectDetailData struct {
	ProductID     string  `json:"productId"`
	Qty           int32   `json:"qty"`
	SubPrice      float64 `json:"subPrice"`
	SubtotalPrice float64 `json:"subtotalPrice"`
	Currency      string  `json:"currency"`
	ProductName   string  `json:"productName"`
}

type SalesDefectResponse struct {
	ResponseCode    string                  `json:"responseCode"`
	ResponseMessage string                  `json:"responseMessage"`
	Data            []SalesDefectDetailData `json:"data"`
}

// fetchProductDetails fetches product information from the external product API
func (h *DefectHandler) fetchProductDetails(authHeader string) (map[string]Product, error) {
	productURL := h.cfg.ProductServiceURL + "/products"

	slog.Info("Fetching product details from external API", "url", productURL)

	// Create a new request instead of using http.Get to set headers
	req, err := http.NewRequest("GET", productURL, nil)
	if err != nil {
		slog.Error("Failed to create request", "error", err, "url", productURL)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add Content-Type and Accept headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Add Authorization header if provided
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("Failed to read product API response", "error", err)
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var products []Product
	if err := json.Unmarshal(body, &products); err != nil {
		slog.Error("Failed to unmarshal product data", "error", err)
		return nil, fmt.Errorf("failed to unmarshal products: %w", err)
	}

	// Create a map for quick lookup by product ID
	productMap := make(map[string]Product)
	for _, product := range products {
		productMap[product.ID] = product
	}

	slog.Info("Successfully fetched products", "count", len(products))
	return productMap, nil
}

func (h *DefectHandler) GetSalesDefect(c *gin.Context) {
	ctx := c.Request.Context()

	// Extract timezone from context if set by middleware
	if loc, exists := c.Get("timezone"); exists {
		if l, ok := loc.(*time.Location); ok {
			slog.Info("Handler passing timezone to context", "location", l.String())
			ctx = context.WithValue(ctx, "timezone", l)
		}
	}

	// Extract merchant_id from context (set by AuthMiddleware)
	merchantID := c.GetString("merchant_id")
	if merchantID == "" {
		merchantID = c.GetString("merchantId")
	}

	// Fallback to manual token parsing if not in context
	if merchantID == "" {
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			tokenString := ""
			if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
				tokenString = authHeader[7:]
			} else {
				tokenString = authHeader
			}

			parts := strings.Split(tokenString, ".")
			if len(parts) == 3 {
				payload, err := base64.RawURLEncoding.DecodeString(parts[1])
				if err == nil {
					var claims map[string]interface{}
					if err := json.Unmarshal(payload, &claims); err == nil {
						if mID, ok := claims["merchant_id"].(string); ok {
							merchantID = mID
						} else if mID, ok := claims["merchantId"].(string); ok {
							merchantID = mID
						} else if sub, ok := claims["sub"].(string); ok {
							merchantID = sub
						}
					}
				}
			}
		}
	}

	if merchantID == "" {
		slog.Error("Merchant ID not found in context or token")
		c.JSON(http.StatusUnauthorized, gin.H{
			"responseCode":    "401",
			"responseMessage": "Unauthorized: Merchant ID not found",
		})
		return
	}

	slog.Info("Fetching aggregated sales defect details", "merchant_id", merchantID)

	// Get data from repository
	repoDetails, err := h.stockRepo.GetSalesDefectDetails(ctx, merchantID)
	if err != nil {
		slog.Error("Failed to fetch aggregated sales defect details", "error", err, "merchant_id", merchantID)
		c.JSON(http.StatusInternalServerError, gin.H{
			"responseCode":    "500",
			"responseMessage": "Internal server error",
		})
		return
	}

	// Fetch product details from external API
	authHeader := c.GetHeader("Authorization")
	productMap, err := h.fetchProductDetails(authHeader)
	if err != nil {
		slog.Warn("Failed to fetch product details, continuing without product info", "error", err)
		productMap = make(map[string]Product)
	}

	// Format response data
	data := make([]SalesDefectDetailData, len(repoDetails))
	for i, d := range repoDetails {
		data[i] = SalesDefectDetailData{
			ProductID:     d.ProductID,
			Qty:           d.Qty,
			SubPrice:      d.SubPrice,
			SubtotalPrice: d.SubtotalPrice,
			Currency:      d.Currency,
		}

		// Populate product name
		if product, found := productMap[d.ProductID]; found {
			data[i].ProductName = product.Name
		} else {
			slog.Warn("Product not found in product service", "product_id", d.ProductID)
		}
	}

	// Sort by product name (case-insensitive)
	sort.Slice(data, func(i, j int) bool {
		return strings.ToLower(data[i].ProductName) < strings.ToLower(data[j].ProductName)
	})

	response := SalesDefectResponse{
		ResponseCode:    "200",
		ResponseMessage: "success",
		Data:            data,
	}

	c.JSON(http.StatusOK, response)
}

func (h *DefectHandler) DeleteSalesDefect(c *gin.Context) {
	productID := c.Param("productId")
	if productID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Product ID is required"})
		return
	}

	// Extract claims from Authorization header (same logic as CreateDefect)
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

	// Parse JWT manually (assuming no middleware sets it yet or following existing pattern)
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		slog.Error("Invalid token format")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token format"})
		return
	}

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
		if mID, ok := claims["merchant_id"].(string); ok {
			merchantID = mID
		} else if mID, ok := claims["merchantId"].(string); ok {
			merchantID = mID
		}
	}
	if merchantID == "" {
		merchantID = userID
	}

	slog.Info("Processing soft delete for sales defect", "product_id", productID, "merchant_id", merchantID, "user_id", userID)

	err = h.stockRepo.DeleteSalesDefectDetail(c.Request.Context(), productID, merchantID, userID)
	if err != nil {
		slog.Error("Failed to delete sales defect", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete sales defect record"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Sales defect record deleted and stock re-added successfully",
		"status":  "success",
	})
}
