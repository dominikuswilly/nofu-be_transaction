package handler

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"nofu-be_transaction/internal/config"
	"nofu-be_transaction/internal/repository"
)

type SalesHistoryHandler struct {
	DB        *pgxpool.Pool
	StockRepo *repository.StockRepository
	Config    *config.Config
}

func NewSalesHistoryHandler(db *pgxpool.Pool, stockRepo *repository.StockRepository, cfg *config.Config) *SalesHistoryHandler {
	return &SalesHistoryHandler{
		DB:        db,
		StockRepo: stockRepo,
		Config:    cfg,
	}
}

type SalesHistoryResponse struct {
	ResponseCode    string           `json:"responseCode"`
	ResponseMessage string           `json:"responseMessage"`
	Data            SalesHistoryData `json:"data"`
}

type SalesHistoryData struct {
	MerchantID  string               `json:"merchantId"`
	SalesDetail []SalesDetailHistory `json:"salesDetail"`
}

type SalesDetailHistory struct {
	ProductID     string `json:"productId"`
	TotalQuantity int32  `json:"totalQuantity"`
	MerchantID    string `json:"merchantId"`
	CreatedBy     string `json:"createdBy"`
	ProductName   string `json:"productName,omitempty"`
	ProductImage  string `json:"productImage,omitempty"`
}

// fetchProductDetails fetches product information from the external product API
func (h *SalesHistoryHandler) fetchProductDetails() (map[string]Product, error) {
	productURL := h.Config.ProductServiceURL + "/products"

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

func (h *SalesHistoryHandler) GetSalesHistory(c *gin.Context) {
	ctx := c.Request.Context()
	timeParam := c.Query("time")

	slog.Info("GetSalesHistory called", "time", timeParam)

	// Validate time parameter
	if timeParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"responseCode":    "400",
			"responseMessage": "time parameter is required (today, week, or month)",
		})
		return
	}

	if timeParam != "today" && timeParam != "week" && timeParam != "month" {
		c.JSON(http.StatusBadRequest, gin.H{
			"responseCode":    "400",
			"responseMessage": "invalid time parameter. Must be: today, week, or month",
		})
		return
	}

	// 1. Extract info from Authorization header (manual parsing fallback)
	authHeader := c.GetHeader("Authorization")
	userID := ""
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
					if sub, ok := claims["sub"].(string); ok {
						userID = sub
					}
				}
			}
		}
	}

	// 2. Extract from context (set by AuthMiddleware)
	merchantID := ""

	// Log context keys individually to avoid JSON errors
	for k, v := range c.Keys {
		slog.Debug("Context key found", "key", k, "type", fmt.Sprintf("%T", v))
	}

	// Try merchant_id from context
	if val, exists := c.Get("merchant_id"); exists {
		if strVal, ok := val.(string); ok {
			merchantID = strVal
		} else if floatVal, ok := val.(float64); ok {
			merchantID = fmt.Sprintf("%.0f", floatVal)
		}
	}

	if merchantID == "" {
		if val, exists := c.Get("merchantId"); exists {
			if strVal, ok := val.(string); ok {
				merchantID = strVal
			} else if floatVal, ok := val.(float64); ok {
				merchantID = fmt.Sprintf("%.0f", floatVal)
			}
		}
	}

	// Final fallback to userID (from token sub claim)
	if merchantID == "" {
		merchantID = userID
	}

	if merchantID == "" {
		slog.Error("merchant_id not found in context or token")
		c.JSON(http.StatusUnauthorized, gin.H{
			"responseCode":    "401",
			"responseMessage": "merchant_id not found in token",
		})
		return
	}

	slog.Info("Fetching sales history", "merchant_id", merchantID, "time", timeParam)

	// Get sales history from repository
	repoSalesDetails, err := h.StockRepo.GetSalesHistory(ctx, merchantID, timeParam)
	if err != nil {
		slog.Error("Failed to get sales history", "error", err, "merchant_id", merchantID, "time", timeParam)
		c.JSON(http.StatusInternalServerError, gin.H{
			"responseCode":    "500",
			"responseMessage": "internal server error",
		})
		return
	}

	// Fetch product details from external API
	productMap, err := h.fetchProductDetails()
	if err != nil {
		slog.Warn("Failed to fetch product details, continuing without product info", "error", err)
		// Continue without product details - they will be empty strings
		productMap = make(map[string]Product)
	}

	// Convert repository type to response type and populate product details
	salesDetails := make([]SalesDetailHistory, len(repoSalesDetails))
	for i, detail := range repoSalesDetails {
		salesDetails[i] = SalesDetailHistory{
			ProductID:     detail.ProductID,
			TotalQuantity: detail.TotalQuantity,
			MerchantID:    detail.MerchantID,
			CreatedBy:     detail.CreatedBy,
		}

		// Populate product name and image
		if product, found := productMap[detail.ProductID]; found {
			salesDetails[i].ProductName = product.Name
			salesDetails[i].ProductImage = product.URL
		} else {
			slog.Warn("Product not found in product service", "product_id", detail.ProductID)
		}
	}

	// Sort by product name (case-insensitive)
	sort.Slice(salesDetails, func(i, j int) bool {
		return strings.ToLower(salesDetails[i].ProductName) < strings.ToLower(salesDetails[j].ProductName)
	})

	response := SalesHistoryResponse{
		ResponseCode:    "200",
		ResponseMessage: "success",
		Data: SalesHistoryData{
			MerchantID:  merchantID,
			SalesDetail: salesDetails,
		},
	}

	c.JSON(http.StatusOK, response)
}
