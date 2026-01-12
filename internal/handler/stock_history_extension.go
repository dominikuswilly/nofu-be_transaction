package handler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type StockHistoryResponse struct {
	ResponseCode    string             `json:"responseCode"`
	ResponseMessage string             `json:"responseMessage"`
	Data            []StockHistoryData `json:"data"`
}

type StockHistoryData struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	CreatedBy string `json:"createdBy"`
	CreatedAt string `json:"createdAt"`
}

func (h *StockHandler) GetStockHistory(c *gin.Context) {
	ctx := c.Request.Context()
	stockMasterID := c.Param("stock_master_id")

	slog.Info("GetStockHistory called", "stock_master_id", stockMasterID)

	query := `
		select t.c_id , t.c_status , t.c_stock_master_id , t.c_created_by , t.ts_created_at 
		from stock_master_history t 
		where t.c_stock_master_id = $1
		order by t.ts_created_at desc
	`

	rows, err := h.DB.Query(ctx, query, stockMasterID)
	if err != nil {
		slog.Error("Failed to query stock master history", "error", err, "stock_master_id", stockMasterID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	defer rows.Close()

	var historyData []StockHistoryData
	for rows.Next() {
		var hd StockHistoryData
		var createdAt time.Time
		var stockMasterIDDb string // we select it but don't need it in response struct based on requirements, but wait, the query selects it.

		// select t.c_id , t.c_status , t.c_stock_master_id , t.c_created_by , t.ts_created_at
		if err := rows.Scan(&hd.ID, &hd.Status, &stockMasterIDDb, &hd.CreatedBy, &createdAt); err != nil {
			slog.Error("Failed to scan stock master history row", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}
		hd.CreatedAt = createdAt.Format("2006-01-02 15:04:05")
		historyData = append(historyData, hd)
	}

	if historyData == nil {
		historyData = []StockHistoryData{}
	}

	c.JSON(http.StatusOK, StockHistoryResponse{
		ResponseCode:    "200",
		ResponseMessage: "success",
		Data:            historyData,
	})
}
