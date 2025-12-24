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
	MerchantID  string        `json:"merchant_id"`
	GivenBy     string        `json:"given_by"`
	StockDetail []StockDetail `json:"stock_detail"`
	CreatedAt   string        `json:"created_at"`
}

type StockDetail struct {
	ID        string `json:"id"`
	ProductID string `json:"product_id"`
	PriceSell string `json:"price_sell"`
}

func (h *StockHandler) GetStock(c *gin.Context) {
	ctx := c.Request.Context()
	merchantID := c.Query("merchant_id")

	if merchantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "merchant_id is required"})
		return
	}

	query := `
		with CTE_STOCK_MASTER as (
			select t.c_id , t.c_admin_id , t.c_merchant_id , t.ts_created_at , t.c_created_by 
			from stock_master t 
			where t.c_merchant_id = $1 AND t.ts_created_at >= CURRENT_DATE AND t.ts_created_at < CURRENT_DATE + INTERVAL '1 day'
		)
		select 
			A.c_id, 
			A.c_product_id, 
			A.d_price, 
			B.c_merchant_id, 
			B.c_created_by,
			B.ts_created_at
		from stock_detail A
		inner join CTE_STOCK_MASTER B on B.c_id = A.c_stock_id 
		where A.c_stock_id in (select A1.c_id from CTE_STOCK_MASTER A1)
	`

	rows, err := h.DB.Query(ctx, query, merchantID)
	if err != nil {
		slog.Error("Failed to query stock", "error", err)
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

		if err := rows.Scan(&id, &productID, &priceSell, &merchantIDDb, &createdByDb, &createdAtDb); err != nil {
			slog.Error("Failed to scan row", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}

		stockDetails = append(stockDetails, StockDetail{
			ID:        id,
			ProductID: productID,
			PriceSell: fmt.Sprintf("%.2f", priceSell),
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
