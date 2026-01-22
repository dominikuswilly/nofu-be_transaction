package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"nofu-be_transaction/internal/config"
	"nofu-be_transaction/internal/repository"
)

type SalesHistoryHandler struct {
	DB        *pgxpool.Pool
	StockRepo *repository.StockRepository
	SalesRepo *repository.SalesRepository
	Config    *config.Config
}

func NewSalesHistoryHandler(db *pgxpool.Pool, stockRepo *repository.StockRepository, salesRepo *repository.SalesRepository, cfg *config.Config) *SalesHistoryHandler {
	return &SalesHistoryHandler{
		DB:        db,
		StockRepo: stockRepo,
		SalesRepo: salesRepo,
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

type SalesHistoryGroupedResponse struct {
	ResponseCode    string                  `json:"responseCode"`
	ResponseMessage string                  `json:"responseMessage"`
	Data            SalesHistoryGroupedData `json:"data"`
}

type SalesHistoryGroupedData struct {
	MerchantID  string                      `json:"merchantId"`
	SalesDetail []SalesGroupedDetailHistory `json:"salesDetail"`
}

type SalesGroupedDetailHistory struct {
	ProductID     string `json:"productId"`
	TotalQuantity int32  `json:"totalQuantity"`
	MerchantID    string `json:"merchantId"`
	CreatedBy     string `json:"createdBy"`
	MinuteBucket  string `json:"minuteBucket"`
	ProductName   string `json:"productName,omitempty"`
	ProductImage  string `json:"productImage,omitempty"`
}

type SalesReportSummaryResponse struct {
	ResponseCode    string                 `json:"responseCode"`
	ResponseMessage string                 `json:"responseMessage"`
	Data            SalesReportSummaryData `json:"data"`
}

type SalesReportSummaryData struct {
	SubtotalPaymentAmount map[string]float64 `json:"subtotalPaymentAmount"`
	TotalPaymentAmount    float64            `json:"totalPaymentAmount"`
}

// fetchProductDetails fetches product information from the external product API using the shared helper
func (h *SalesHistoryHandler) fetchProductDetails(authHeader string) (map[string]Product, error) {
	return fetchProductDetailsInternal(h.Config.ProductServiceURL, authHeader)
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
	repoSalesDetails, err := h.SalesRepo.GetSalesHistory(ctx, merchantID, timeParam)
	if err != nil {
		slog.Error("Failed to get sales history", "error", err, "merchant_id", merchantID, "time", timeParam)
		c.JSON(http.StatusInternalServerError, gin.H{
			"responseCode":    "500",
			"responseMessage": "internal server error",
		})
		return
	}

	// Fetch product details from external API
	productMap, err := h.fetchProductDetails(authHeader)
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

func (h *SalesHistoryHandler) GetSalesHistoryTodayGrouped(c *gin.Context) {
	ctx := c.Request.Context()

	// Extract timezone from context if set by middleware
	if loc, exists := c.Get("timezone"); exists {
		if l, ok := loc.(*time.Location); ok {
			slog.Info("Handler passing timezone to context", "location", l.String())
			ctx = context.WithValue(ctx, "timezone", l)
		} else {
			slog.Warn("Timezone found in Gin context but not of type *time.Location", "type", fmt.Sprintf("%T", loc))
		}
	} else {
		slog.Warn("Timezone not found in Gin context")
	}

	slog.Info("GetSalesHistoryTodayGrouped called")

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

	slog.Info("Fetching grouped sales history", "merchant_id", merchantID)

	// Get grouped sales history from repository
	repoSalesDetails, err := h.SalesRepo.GetSalesHistoryTodayGrouped(ctx, merchantID)
	if err != nil {
		slog.Error("Failed to get grouped sales history", "error", err, "merchant_id", merchantID)
		c.JSON(http.StatusInternalServerError, gin.H{
			"responseCode":    "500",
			"responseMessage": "internal server error",
		})
		return
	}

	// Fetch product details from external API
	productMap, err := h.fetchProductDetails(authHeader)
	if err != nil {
		slog.Warn("Failed to fetch product details, continuing without product info", "error", err)
		productMap = make(map[string]Product)
	}

	// Convert repository type to response type and populate product details
	salesDetails := make([]SalesGroupedDetailHistory, len(repoSalesDetails))
	for i, detail := range repoSalesDetails {
		salesDetails[i] = SalesGroupedDetailHistory{
			ProductID:     detail.ProductID,
			TotalQuantity: detail.TotalQuantity,
			MerchantID:    detail.MerchantID,
			CreatedBy:     detail.CreatedBy,
			MinuteBucket:  detail.MinuteBucket,
		}

		// Populate product name and image
		if product, found := productMap[detail.ProductID]; found {
			salesDetails[i].ProductName = product.Name
			salesDetails[i].ProductImage = product.URL
		} else {
			slog.Warn("Product not found in product service", "product_id", detail.ProductID)
		}
	}

	// Sort by minute bucket (descending) then by product name
	sort.Slice(salesDetails, func(i, j int) bool {
		if salesDetails[i].MinuteBucket != salesDetails[j].MinuteBucket {
			return salesDetails[i].MinuteBucket > salesDetails[j].MinuteBucket
		}
		return strings.ToLower(salesDetails[i].ProductName) < strings.ToLower(salesDetails[j].ProductName)
	})

	response := SalesHistoryGroupedResponse{
		ResponseCode:    "200",
		ResponseMessage: "success",
		Data: SalesHistoryGroupedData{
			MerchantID:  merchantID,
			SalesDetail: salesDetails,
		},
	}

	// Set Content-Type header explicitly (as requested in previous conversations)
	c.Header("Content-Type", "application/json")
	c.JSON(http.StatusOK, response)
}

func (h *SalesHistoryHandler) GetBalance(c *gin.Context) {
	ctx := c.Request.Context()

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
	if val, exists := c.Get("merchant_id"); exists {
		if strVal, ok := val.(string); ok {
			merchantID = strVal
		}
	}

	if merchantID == "" {
		merchantID = userID
	}

	if merchantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"responseCode":    "401",
			"responseMessage": "merchant_id not found in token",
		})
		return
	}

	balance, err := h.SalesRepo.GetMerchantBalance(ctx, merchantID)
	if err != nil {
		slog.Error("Failed to get merchant balance", "error", err, "merchant_id", merchantID)
		c.JSON(http.StatusInternalServerError, gin.H{
			"responseCode":    "500",
			"responseMessage": "internal server error",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"responseCode":    "200",
		"responseMessage": "success",
		"data":            balance,
	})
}

func (h *SalesHistoryHandler) GetSalesReportSummary(c *gin.Context) {
	ctx := c.Request.Context()

	slog.Info("GetSalesReportSummary called")

	// 1. Extract merchantId from query parameter
	merchantID := c.Query("merchantId")
	if merchantID == "" {
		slog.Error("merchantId query parameter is required")
		c.JSON(http.StatusBadRequest, gin.H{
			"responseCode":    "400",
			"responseMessage": "merchantId query parameter is required",
		})
		return
	}

	// Fetch daily sales records from repo
	salesItems, err := h.SalesRepo.GetSalesReportSummaryData(ctx, merchantID)
	if err != nil {
		slog.Error("Failed to fetch sales report summary data", "error", err, "merchant_id", merchantID)
		c.JSON(http.StatusInternalServerError, gin.H{
			"responseCode":    "500",
			"responseMessage": "Internal server error",
		})
		return
	}

	// Business Logic: Calculate subtotals and total
	subtotals := make(map[string]float64)
	var totalAmount float64

	for _, item := range salesItems {
		paymentMethod := strings.ToLower(item.PaymentMethod)
		subtotals[paymentMethod] += item.TotalPayment
		totalAmount += item.TotalPayment
	}

	// Ensure all expected keys are present even if 0 if necessary,
	// but based on example, we just include what's there.

	response := SalesReportSummaryResponse{
		ResponseCode:    "200",
		ResponseMessage: "Success",
		Data: SalesReportSummaryData{
			SubtotalPaymentAmount: subtotals,
			TotalPaymentAmount:    totalAmount,
		},
	}

	c.JSON(http.StatusOK, response)
}
