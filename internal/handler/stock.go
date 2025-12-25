package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type StockHandler struct {
	DB *pgxpool.Pool
}

func NewStockHandler(db *pgxpool.Pool) *StockHandler {
	return &StockHandler{DB: db}
}

type StockResponse struct {
	ResponseCode    string    `json:"responseCode"`
	ResponseMessage string    `json:"responseMessage"`
	Data            StockData `json:"data"`
}

type StockData struct {
	MerchantID  string        `json:"merchantId"`
	GivenBy     string        `json:"givenBy"`
	StockDetail []StockDetail `json:"stockDetail"`
	CreatedAt   string        `json:"createdAt"`
}

type StockDetail struct {
	ID        string `json:"id"`
	ProductID string `json:"productId"`
	PriceSell string `json:"priceSell"`
	Qty       int32  `json:"qty"`
	Currency  string `json:"currency"`
}

func (h *StockHandler) GetStock(c *gin.Context) {
	ctx := c.Request.Context()
	merchantID := c.Query("merchant_id")
	dateParam := c.Query("date") // Optional: format YYYY-MM-DD

	slog.Info("GetStock called", "merchant_id", merchantID, "date", dateParam)

	if merchantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "merchant_id is required"})
		return
	}

	// Validate date format if provided
	if dateParam != "" {
		_, err := time.Parse("2006-01-02", dateParam)
		if err != nil {
			slog.Error("Invalid date format", "date", dateParam, "error", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date format, use YYYY-MM-DD"})
			return
		}
	}

	var query string
	var args []interface{}

	if dateParam != "" {
		// Use provided date
		query = `
			with CTE_STOCK_MASTER as (
				select t.c_id , t.c_admin_id , t.c_merchant_id , t.ts_created_at , t.c_created_by 
				from stock_master t 
				where t.c_merchant_id = $1 AND DATE(t.ts_created_at) = $2
			)
			select 
				A.c_id, 
				A.c_product_id, 
				A.d_price, 
				A.i_qty,
				A.c_currency,
				B.c_merchant_id, 
				B.c_created_by,
				B.ts_created_at
			from stock_detail A
			inner join CTE_STOCK_MASTER B on B.c_id = A.c_stock_id 
			where A.c_stock_id in (select A1.c_id from CTE_STOCK_MASTER A1)
		`
		args = []interface{}{merchantID, dateParam}
	} else {
		query = `
			with CTE_STOCK_MASTER as (
				select t.c_id , t.c_admin_id , t.c_merchant_id , t.ts_created_at , t.c_created_by 
				from stock_master t 
				where t.c_merchant_id = $1 
				AND DATE(t.ts_created_at) = CURRENT_DATE
			)
			select 
				A.c_id, 
				A.c_product_id, 
				A.d_price, 
				A.i_qty,
				A.c_currency,
				B.c_merchant_id, 
				B.c_created_by,
				B.ts_created_at
			from stock_detail A
			inner join CTE_STOCK_MASTER B on B.c_id = A.c_stock_id 
			where A.c_stock_id in (select A1.c_id from CTE_STOCK_MASTER A1)
		`
		args = []interface{}{merchantID}
	}

	slog.Info("Executing stock query", "merchant_id", merchantID, "date", dateParam)
	rows, err := h.DB.Query(ctx, query, args...)
	if err != nil {
		slog.Error("Failed to query stock", "error", err, "merchant_id", merchantID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	defer rows.Close()

	var stockDetails []StockDetail
	var merchantIDDb, createdByDb string
	var createdAtDb time.Time

	firstRow := true

	for rows.Next() {
		var id, productID string
		var priceSell float64
		var qty int32
		var currency string

		if err := rows.Scan(&id, &productID, &priceSell, &qty, &currency, &merchantIDDb, &createdByDb, &createdAtDb); err != nil {
			slog.Error("Failed to scan row", "error", err, "merchant_id", merchantID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}

		slog.Info("Scanned stock row", "id", id, "product_id", productID, "qty", qty, "merchant_id", merchantIDDb)

		stockDetails = append(stockDetails, StockDetail{
			ID:        id,
			ProductID: productID,
			PriceSell: fmt.Sprintf("%.2f", priceSell),
			Qty:       qty,
			Currency:  currency,
		})

		firstRow = false
	}

	if err := rows.Err(); err != nil {
		slog.Error("Rows iteration error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	if stockDetails == nil {
		stockDetails = []StockDetail{}
	}

	slog.Info("Stock query completed", "merchant_id", merchantID, "row_count", len(stockDetails), "first_row", firstRow)

	response := StockResponse{
		ResponseCode:    "200",
		ResponseMessage: "success",
		Data: StockData{
			MerchantID:  merchantIDDb,
			GivenBy:     createdByDb,
			StockDetail: stockDetails,
			CreatedAt:   createdAtDb.Format("2006-01-02 15:04:05"),
		},
	}

	if firstRow {
		response.Data.MerchantID = merchantID
		response.Data.CreatedAt = ""
	}

	c.JSON(http.StatusOK, response)
}
