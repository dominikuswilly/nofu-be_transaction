package models

import "time"

// StockRestockMaster represents the sales_restock_master table
type StockRestockMaster struct {
	CID         string // c_id - UUID v7
	CMerchantID string // c_merchant_id - by authorization token (claims.sub)
	CStatus     string // c_status - pending
}

// StockRestockDetail represents the sales_restock_detail table
type StockRestockDetail struct {
	CID             string // c_id - UUID v7
	CStockRestockID string // c_stock_restock_id - based on sales_restock_master.id
	CProductID      string // c_product_id - based on request body
	IQty            int    // i_qty - based on request body
	CCreatedBy      string // c_created_by - based on authorization token (claims.sub)
}

// StockRestockHistory represents the sales_restock_history table
type StockRestockHistory struct {
	CID             string    // c_id - UUID v7
	CStockRestockID string    // c_stock_restock_id - based on sales_restock_master.id
	ISeq            int       // i_seq - sequential started from 1
	CStatus         string    // c_status - initiate "PENDING"
	CCreatedBy      string    // c_created_by - based on authorization token (claims.sub)
	TsCreatedAt     time.Time // ts_created_at - timestamp
}
