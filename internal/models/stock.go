package models

import "time"

// StockMaster represents the stock_master table
type StockMaster struct {
	CID         string    // c_id - UUID v7
	CCreatedBy  string    // c_created_by - based on authorization claims.sub
	CMerchantID string    // c_merchant_id - from request body
	CAdminID    string    // c_admin_id - based on authorization claims.sub
	TsCreatedAt time.Time // ts_created_at - auto-generated
}

// StockDetail represents the stock_detail table
type StockDetail struct {
	CID        string  // c_id - UUID v7
	CStockID   string  // c_stock_id - foreign key to stock_master.c_id
	CProductID string  // c_product_id - from request
	DPrice     float64 // d_price - price
	IQty       int32   // i_qty - quantity
	CCurrency  string  // c_currency - currency code
}

// StockMessage represents the incoming RabbitMQ message
type StockMessage struct {
	UserID       string               `json:"userId"`       // For c_created_by and c_admin_id
	MerchantID   string               `json:"merchantId"`   // For c_merchant_id
	StockDetails []StockDetailMessage `json:"stockDetails"` // Stock items
}

// StockDetailMessage represents individual stock items in the message
type StockDetailMessage struct {
	ProductID string  `json:"productId"`
	Price     float64 `json:"price"`
	Qty       int32   `json:"qty"`
	Currency  string  `json:"currency"`
}
