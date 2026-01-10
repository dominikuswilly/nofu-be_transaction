package models

import "time"

// StockMaster represents the stock_master table
type StockMaster struct {
	CID            string    // c_id - UUID v7
	CCreatedBy     string    // c_created_by - based on authorization claims.sub
	CMerchantID    string    // c_merchant_id - from request body
	CAdminID       string    // c_admin_id - based on authorization claims.sub
	CPaymentMethod string    // c_payment_method
	DTotalPayment  float64   // d_total_payment
	CStatus        string    // c_status
	CUpdatedBy     string    // c_updated_by
	TsUpdatedAt    time.Time // ts_updated_at
	CDeletedBy     string    // c_deleted_by
	TsDeletedAt    time.Time // ts_deleted_at
	TsCreatedAt    time.Time // ts_created_at - auto-generated
}

// StockDetail represents the stock_detail table
type StockDetail struct {
	CID         string  // c_id - UUID v7
	CStockID    string  // c_stock_id - foreign key to stock_master.c_id
	CProductID  string  // c_product_id - from request
	DPrice      float64 // d_price - price
	IQty        int32   // i_qty - quantity
	IQtyCurrent int32   // i_qty_current - current quantity
	IQtyRestock int32   // i_qty_restock - restock quantity
	CCurrency   string  // c_currency - currency code
}

// StockMessage represents the incoming RabbitMQ message
type StockMessage struct {
	UserID       string               `json:"userId"`       // For c_created_by and c_admin_id
	MerchantID   string               `json:"merchantId"`   // For c_merchant_id
	Status       string               `json:"status"`       // For c_status
	StockDetails []StockDetailMessage `json:"stockDetails"` // Stock items
}

// StockDetailMessage represents individual stock items in the message
type StockDetailMessage struct {
	ProductID string  `json:"productId"`
	Price     float64 `json:"price"`
	Qty       int32   `json:"qty"`
	Currency  string  `json:"currency"`
}

// SalesMaster represents the sales_master table
type SalesMaster struct {
	CID            string    // c_id - UUID v7
	TsCreatedAt    time.Time // ts_created_at - auto-generated
	CCreatedBy     string    // c_created_by - based on authorization claims.sub
	CMerchantID    string    // c_merchant_id - from request body
	CPaymentMethod string    // c_payment_method
	DTotalPayment  float64   // d_total_payment
}

// SalesDetail represents the sales_detail table
type SalesDetail struct {
	CID            string  // c_id - UUID v7
	CSalesID       string  // c_sales_id - foreign key to sales_master.c_id
	CProductID     string  // c_product_id - from request
	IQty           int32   // i_qty - quantity sold
	DPrice         float64 // d_price - numeric(10,4)
	CCurrency      string  // c_currency - currency code
	CStockDetailID string  // c_stock_detail_id - foreign key to stock_detail.c_stock_detail_id
}

// SalesMessage represents the incoming RabbitMQ message for sales
type SalesMessage struct {
	UserID        string               `json:"userId"`        // For c_created_by
	MerchantID    string               `json:"merchantId"`    // For c_merchant_id
	PaymentMethod string               `json:"paymentMethod"` // For c_payment_method
	TotalPayment  float64              `json:"totalPayment"`  // For d_total_payment
	SalesDetails  []SalesDetailMessage `json:"salesDetails"`  // Sales items
}

// SalesDetailMessage represents individual sales items in the message
type SalesDetailMessage struct {
	ProductID     string  `json:"productId"`
	Qty           int32   `json:"qty"`
	Price         float64 `json:"price"`
	Currency      string  `json:"currency"`
	StockDetailID string  `json:"stockDetailId"`
}

// SalesDefectMaster represents the sales_defect_master table
type SalesDefectMaster struct {
	CID         string    // c_id - UUID v7
	CCreatedBy  string    // c_created_by - based on authorization claims.sub
	TsCreatedAt time.Time // ts_created_at - auto-generated
	CUpdatedBy  string    // c_updated_by
	TsUpdatedAt time.Time // ts_updated_at
	CDeletedBy  string    // c_deleted_by
	TsDeletedAt time.Time // ts_deleted_at
	CMerchantID string    // c_merchant_id - added as requested
}

// SalesDefectDetail represents the sales_defect_detail table
type SalesDefectDetail struct {
	CID            string    // c_id - UUID v7
	CSalesDefectID string    // c_sales_defect_id - foreign key to sales_defect_master.c_id
	CProductID     string    // c_product_id
	IQty           int32     // i_qty
	DPrice         float64   // d_price
	CCurrency      string    // c_currency
	CStockDetailID string    // c_stock_detail_id
	TsCreatedAt    time.Time // ts_created_at
	TsDeletedAt    time.Time // ts_deleted_at
	CDeletedBy     string    // c_deleted_by
}

// SalesDefectMessage represents the incoming defect message
type SalesDefectMessage struct {
	UserID        string                     `json:"userId"`
	MerchantID    string                     `json:"merchantId"`
	DefectDetails []SalesDefectDetailMessage `json:"defectDetails"`
}

// SalesDefectDetailMessage represents individual defect items in the message
type SalesDefectDetailMessage struct {
	ProductID     string  `json:"productId"`
	Qty           int32   `json:"qty"`
	Price         float64 `json:"price"`
	Currency      string  `json:"currency"`
	StockDetailID string  `json:"stockDetailId"`
}
