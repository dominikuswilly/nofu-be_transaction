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
	ID               string  `json:"id"`
	MerchantID       string  `json:"merchantId"`
	MerchantUsername string  `json:"merchantUsername"`
	MerchantName     string  `json:"merchantName"`
	Status           string  `json:"status"`
	CreatedBy        string  `json:"createdBy"`
	CreatedAt        string  `json:"createdAt"`
	UpdatedBy        string  `json:"updatedBy"`
	UpdatedAt        string  `json:"updatedAt"`
	Longitude        float64 `json:"longitude"`
	Latitude         float64 `json:"latitude"`
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

type RestockHistoryData struct {
	ID        string `json:"id"`
	Seq       int    `json:"seq"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
	CreatedBy string `json:"createdBy"`
}

type RestockHistoryResponse struct {
	ResponseCode    string               `json:"responseCode"`
	ResponseMessage string               `json:"responseMessage"`
	Data            []RestockHistoryData `json:"data"`
}

type RestockIDDetailItem struct {
	ID              string `json:"id"`
	ProductName     string `json:"productName"`
	ProductID       string `json:"productId"`
	ProductImageUrl string `json:"productImageUrl"`
	Qty             int    `json:"qty"`
}

type RestockByIDLocation struct {
	Longitude float64 `json:"longitude"`
	Latitude  float64 `json:"latitude"`
}

type RestockByIDData struct {
	ID            string                `json:"id"`
	TotalQty      int                   `json:"totalQty"`
	TotalItem     int                   `json:"totalItem"`
	Status        string                `json:"status"`
	Location      RestockByIDLocation   `json:"location"`
	RestockDetail []RestockIDDetailItem `json:"restockDetail"`
}

type RestockByIDResponse struct {
	ResponseCode    string          `json:"responseCode"`
	ResponseMessage string          `json:"responseMessage"`
	Data            RestockByIDData `json:"data"`
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

	// Fetch merchant details to get name
	var merchantNm string
	merchant, err := fetchMerchantDetails(h.cfg.CustomerServiceURL, merchantID, authHeader)
	if err != nil {
		slog.Warn("Failed to fetch merchant details during restock creation", "error", err, "merchantID", merchantID)
		// We can proceed with empty name or Handle error.
		// For now, proceeding with empty name as it might be better than failing the whole transaction if just name is missing,
		// but typically if it's required for display we might want it.
		// Given the user asked to store it, let's assume valid merchant should have it.
	} else if merchant != nil {
		merchantNm = merchant.Name
	}

	// Prepare data
	masterID, _ := uuid.NewV7()
	now := time.Now()
	master := &models.StockRestockMaster{
		CID:         masterID.String(),
		CMerchantID: merchantID,
		CMerchantNm: merchantNm,
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

	// Get timezone from context
	loc := time.UTC
	if val, exists := c.Get("timezone"); exists {
		if l, ok := val.(*time.Location); ok {
			loc = l
		}
	}

	// Fetch merchant details for GetRestock (all restocks belong to the same merchant)
	var merchName, merchUsername string
	if len(restocks) > 0 {
		merchant, err := fetchMerchantDetails(h.cfg.CustomerServiceURL, merchantID, authHeader)
		if err != nil {
			slog.Warn("Failed to fetch merchant details, continuing with empty merchant info", "error", err, "merchantID", merchantID)
		} else if merchant != nil {
			merchName = merchant.Name
			merchUsername = merchant.Username
		}
	}

	data := make([]RestockStatusData, len(restocks))
	for i, r := range restocks {
		updatedAt := ""
		if r.TsUpdatedAt != nil {
			updatedAt = r.TsUpdatedAt.In(loc).Format("2006-01-02T15:04:05")
		}

		data[i] = RestockStatusData{
			ID:               r.CID,
			MerchantID:       r.CMerchantID,
			MerchantUsername: merchUsername,
			MerchantName:     merchName,
			Status:           r.CStatus,
			CreatedBy:        r.CCreatedBy,
			CreatedAt:        r.TsCreatedAt.In(loc).Format("2006-01-02T15:04:05"),
			UpdatedBy:        r.CUpdatedBy,
			UpdatedAt:        updatedAt,
			Longitude:        r.DLongitude,
			Latitude:         r.DLatitude,
		}
	}

	c.JSON(http.StatusOK, RestockStatusResponse{
		ResponseCode:    "200",
		ResponseMessage: "success",
		Data:            data,
	})
}

// fetchProductDetails fetches product information from the external product API using the shared helper
func (h *RestockHandler) fetchProductDetails(authHeader string) (map[string]Product, error) {
	return fetchProductDetailsInternal(h.cfg.ProductServiceURL, authHeader)
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

func (h *RestockHandler) GetRestockByID(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"responseCode": "400", "responseMessage": "ID is required"})
		return
	}

	// 1. Fetch master record
	master, err := h.repo.GetRestockByID(c.Request.Context(), id)
	if err != nil {
		slog.Error("Failed to fetch restock master", "error", err, "id", id)
		c.JSON(http.StatusInternalServerError, gin.H{
			"responseCode":    "500",
			"responseMessage": "Failed to fetch restock info",
		})
		return
	}
	if master == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"responseCode":    "404",
			"responseMessage": "Restock not found",
		})
		return
	}

	// 2. Fetch detail records
	details, err := h.repo.GetRestockDetail(c.Request.Context(), id)
	if err != nil {
		slog.Error("Failed to fetch restock details", "error", err, "id", id)
		c.JSON(http.StatusInternalServerError, gin.H{
			"responseCode":    "500",
			"responseMessage": "Failed to fetch restock details",
		})
		return
	}

	// 3. Fetch product details from external API
	authHeader := c.GetHeader("Authorization")
	productMap, err := h.fetchProductDetails(authHeader)
	if err != nil {
		slog.Warn("Failed to fetch product details, continuing without full product info", "error", err)
		productMap = make(map[string]Product)
	}

	// 4. Map and calculate
	restockDetails := make([]RestockIDDetailItem, len(details))
	totalQty := 0
	for i, d := range details {
		productName := ""
		productImageUrl := ""
		if p, found := productMap[d.CProductID]; found {
			productName = p.Name
			productImageUrl = p.URL // Mapping URL to productImageUrl
		}

		restockDetails[i] = RestockIDDetailItem{
			ID:              d.CID,
			ProductName:     productName,
			ProductID:       d.CProductID,
			ProductImageUrl: productImageUrl,
			Qty:             d.IQty,
		}
		totalQty += d.IQty
	}

	response := RestockByIDResponse{
		ResponseCode:    "200",
		ResponseMessage: "success",
		Data: RestockByIDData{
			ID:        master.CID,
			TotalQty:  totalQty,
			TotalItem: len(details),
			Status:    master.CStatus,
			Location: RestockByIDLocation{
				Longitude: master.DLongitude,
				Latitude:  master.DLatitude,
			},
			RestockDetail: restockDetails,
		},
	}

	c.JSON(http.StatusOK, response)
}

func (h *RestockHandler) GetRestockHistory(c *gin.Context) {
	restockID := c.Param("id")
	if restockID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"responseCode": "400", "responseMessage": "Restock ID is required"})
		return
	}

	history, err := h.repo.GetRestockHistory(c.Request.Context(), restockID)
	if err != nil {
		slog.Error("Failed to fetch restock history", "error", err, "restockID", restockID)
		c.JSON(http.StatusInternalServerError, gin.H{
			"responseCode":    "500",
			"responseMessage": "Failed to fetch restock history",
		})
		return
	}

	// Get timezone from context
	loc := time.UTC
	if val, exists := c.Get("timezone"); exists {
		if l, ok := val.(*time.Location); ok {
			loc = l
		}
	}

	data := make([]RestockHistoryData, len(history))
	for i, item := range history {
		data[i] = RestockHistoryData{
			ID:        item.CID,
			Seq:       item.ISeq,
			Status:    item.CStatus,
			CreatedAt: item.TsCreatedAt.In(loc).Format("2006-01-02T15:04:05"),
			CreatedBy: item.CCreatedBy,
		}
	}

	c.JSON(http.StatusOK, RestockHistoryResponse{
		ResponseCode:    "200",
		ResponseMessage: "success",
		Data:            data,
	})
}

func (h *RestockHandler) GetAdminRestock(c *gin.Context) {
	// Get optional time range query parameters
	timeStart := c.Query("time_start")
	timeEnd := c.Query("time_end")

	// Validate date format if provided
	if timeStart != "" {
		if _, err := time.Parse("2006-01-02", timeStart); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"responseCode":    "400",
				"responseMessage": "Invalid time_start format. Use YYYY-MM-DD",
			})
			return
		}
	}
	if timeEnd != "" {
		if _, err := time.Parse("2006-01-02", timeEnd); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"responseCode":    "400",
				"responseMessage": "Invalid time_end format. Use YYYY-MM-DD",
			})
			return
		}
	}

	restocks, err := h.repo.GetAllRestock(c.Request.Context(), timeStart, timeEnd)
	if err != nil {
		slog.Error("Failed to fetch admin restock status", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"responseCode":    "500",
			"responseMessage": "Failed to fetch admin restock status",
		})
		return
	}

	// Get timezone from context
	loc := time.UTC
	if val, exists := c.Get("timezone"); exists {
		if l, ok := val.(*time.Location); ok {
			loc = l
		}
	}

	// Fetch merchant details (GetAdminRestock might have multiple merchants)
	authHeader := c.GetHeader("Authorization")
	merchantCache := make(map[string]*Merchant)

	data := make([]RestockStatusData, len(restocks))
	for i, r := range restocks {
		updatedAt := ""
		if r.TsUpdatedAt != nil {
			updatedAt = r.TsUpdatedAt.In(loc).Format("2006-01-02T15:04:05")
		}

		merchName := ""
		merchUsername := ""

		if r.CMerchantID != "" {
			if merchant, ok := merchantCache[r.CMerchantID]; ok {
				if merchant != nil {
					merchName = merchant.Name
					merchUsername = merchant.Username
				}
			} else {
				merchant, err := fetchMerchantDetails(h.cfg.CustomerServiceURL, r.CMerchantID, authHeader)
				if err != nil {
					slog.Warn("Failed to fetch merchant details for admin view", "error", err, "merchantID", r.CMerchantID)
					merchantCache[r.CMerchantID] = nil // Cache nil to avoid repeated failed calls
				} else {
					merchantCache[r.CMerchantID] = merchant
					if merchant != nil {
						merchName = merchant.Name
						merchUsername = merchant.Username
					}
				}
			}
		}

		data[i] = RestockStatusData{
			ID:               r.CID,
			MerchantID:       r.CMerchantID,
			MerchantUsername: merchUsername,
			MerchantName:     merchName,
			Status:           r.CStatus,
			CreatedBy:        r.CCreatedBy,
			CreatedAt:        r.TsCreatedAt.In(loc).Format("2006-01-02T15:04:05"),
			UpdatedBy:        r.CUpdatedBy,
			UpdatedAt:        updatedAt,
			Longitude:        r.DLongitude,
			Latitude:         r.DLatitude,
		}
	}

	c.JSON(http.StatusOK, RestockStatusResponse{
		ResponseCode:    "200",
		ResponseMessage: "success",
		Data:            data,
	})
}

func (h *RestockHandler) PatchRestock(c *gin.Context) {
	restockID := c.Param("id")
	if restockID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"responseCode": "400", "responseMessage": "Restock ID is required"})
		return
	}

	var req struct {
		Action string `json:"action" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"responseCode":    "400",
			"responseMessage": "Invalid request body: " + err.Error(),
		})
		return
	}

	// Validate action
	if req.Action != "approve" && req.Action != "reject" {
		c.JSON(http.StatusBadRequest, gin.H{
			"responseCode":    "400",
			"responseMessage": "Invalid action. Must be 'approve' or 'reject'",
		})
		return
	}

	// Extract claims from Authorization header (manual extraction since this might hit an unprotected route group initially or needs custom logic)
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

	adminID, _ := claims["sub"].(string)
	if adminID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"responseCode": "401", "responseMessage": "User ID (sub) not found in token"})
		return
	}

	if req.Action == "approve" {
		if err := h.repo.ApproveRestock(c.Request.Context(), restockID, adminID); err != nil {
			if strings.Contains(err.Error(), "not eligible") {
				c.JSON(http.StatusForbidden, gin.H{
					"responseCode":    "403",
					"responseMessage": "Restock not eligible for approval",
				})
			} else {
				slog.Error("Failed to approve restock", "error", err, "id", restockID)
				c.JSON(http.StatusInternalServerError, gin.H{
					"responseCode":    "500",
					"responseMessage": "Failed to approve restock",
				})
			}
			return
		}
	} else if req.Action == "reject" {
		if err := h.repo.RejectRestock(c.Request.Context(), restockID, adminID); err != nil {
			if strings.Contains(err.Error(), "not eligible") {
				c.JSON(http.StatusForbidden, gin.H{
					"responseCode":    "403",
					"responseMessage": "Restock not eligible for rejection",
				})
			} else {
				slog.Error("Failed to reject restock", "error", err, "id", restockID)
				c.JSON(http.StatusInternalServerError, gin.H{
					"responseCode":    "500",
					"responseMessage": "Failed to reject restock",
				})
			}
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"responseCode":    "200",
		"responseMessage": "success",
	})
}

func (h *RestockHandler) GetAdminRestockHistory(c *gin.Context) {
	// 1. Get date from query param, default to today if empty
	dateStr := c.Query("date")
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	// Validate date format
	_, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"responseCode":    "400",
			"responseMessage": "Invalid date format. Use YYYY-MM-DD",
		})
		return
	}

	// 2. Fetch history from repo
	restocks, err := h.repo.GetRestockHistoryByDate(c.Request.Context(), dateStr)
	if err != nil {
		slog.Error("Failed to fetch admin restock history", "error", err, "date", dateStr)
		c.JSON(http.StatusInternalServerError, gin.H{
			"responseCode":    "500",
			"responseMessage": "Failed to fetch admin restock history",
		})
		return
	}

	// 3. Get timezone
	loc := time.UTC
	if val, exists := c.Get("timezone"); exists {
		if l, ok := val.(*time.Location); ok {
			loc = l
		}
	}

	// 4. Enrich with merchant details
	authHeader := c.GetHeader("Authorization")
	merchantCache := make(map[string]*Merchant)

	data := make([]RestockStatusData, len(restocks))
	for i, r := range restocks {
		updatedAt := ""
		if r.TsUpdatedAt != nil {
			updatedAt = r.TsUpdatedAt.In(loc).Format("2006-01-02T15:04:05")
		}

		merchName := ""
		merchUsername := ""

		if r.CMerchantID != "" {
			if merchant, ok := merchantCache[r.CMerchantID]; ok {
				if merchant != nil {
					merchName = merchant.Name
					merchUsername = merchant.Username
				}
			} else {
				merchant, err := fetchMerchantDetails(h.cfg.CustomerServiceURL, r.CMerchantID, authHeader)
				if err != nil {
					slog.Warn("Failed to fetch merchant details for history view", "error", err, "merchantID", r.CMerchantID)
					merchantCache[r.CMerchantID] = nil
				} else {
					merchantCache[r.CMerchantID] = merchant
					if merchant != nil {
						merchName = merchant.Name
						merchUsername = merchant.Username
					}
				}
			}
		}

		data[i] = RestockStatusData{
			ID:               r.CID,
			MerchantID:       r.CMerchantID,
			MerchantUsername: merchUsername,
			MerchantName:     merchName,
			Status:           r.CStatus,
			CreatedBy:        r.CCreatedBy,
			CreatedAt:        r.TsCreatedAt.In(loc).Format("2006-01-02T15:04:05"),
			UpdatedBy:        r.CUpdatedBy,
			UpdatedAt:        updatedAt,
			Longitude:        r.DLongitude,
			Latitude:         r.DLatitude,
		}
	}

	c.JSON(http.StatusOK, RestockStatusResponse{
		ResponseCode:    "200",
		ResponseMessage: "success",
		Data:            data,
	})
}
