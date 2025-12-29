package repository

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"nofu-be_transaction/internal/models"
)

type StockRepository struct {
	db *pgxpool.Pool
}

func NewStockRepository(db *pgxpool.Pool) *StockRepository {
	return &StockRepository{db: db}
}

// InsertStockMaster inserts a new stock master record
func (r *StockRepository) InsertStockMaster(ctx context.Context, stockMaster *models.StockMaster) error {
	query := `
		INSERT INTO stock_master (c_id, c_created_by, c_merchant_id, c_admin_id, ts_created_at)
		VALUES ($1, $2, $3, $4, $5)
	`

	_, err := r.db.Exec(ctx, query,
		stockMaster.CID,
		stockMaster.CCreatedBy,
		stockMaster.CMerchantID,
		stockMaster.CAdminID,
		stockMaster.TsCreatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to insert stock master: %w", err)
	}

	slog.Info("Inserted stock master", "stock_id", stockMaster.CID, "merchant_id", stockMaster.CMerchantID)
	return nil
}

// InsertStockDetails inserts multiple stock detail records
func (r *StockRepository) InsertStockDetails(ctx context.Context, stockDetails []models.StockDetail) error {
	if len(stockDetails) == 0 {
		return nil
	}

	// Use batch insert for better performance
	batch := &pgx.Batch{}
	query := `
		INSERT INTO stock_detail (c_id, c_stock_id, c_product_id, d_price, i_qty, c_currency)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	for _, detail := range stockDetails {
		batch.Queue(query,
			detail.CID,
			detail.CStockID,
			detail.CProductID,
			detail.DPrice,
			detail.IQty,
			detail.CCurrency,
		)
	}

	br := r.db.SendBatch(ctx, batch)
	defer br.Close()

	// Execute all batched queries
	for i := 0; i < len(stockDetails); i++ {
		_, err := br.Exec()
		if err != nil {
			return fmt.Errorf("failed to insert stock detail at index %d: %w", i, err)
		}
	}

	slog.Info("Inserted stock details", "count", len(stockDetails))
	return nil
}

// InsertStockTransaction inserts stock master and details in a single transaction
func (r *StockRepository) InsertStockTransaction(ctx context.Context, stockMaster *models.StockMaster, stockDetails []models.StockDetail) error {
	// Begin transaction
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) // Rollback if not committed

	// Insert stock master
	masterQuery := `
		INSERT INTO stock_master (c_id, c_created_by, c_merchant_id, c_admin_id, ts_created_at)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err = tx.Exec(ctx, masterQuery,
		stockMaster.CID,
		stockMaster.CCreatedBy,
		stockMaster.CMerchantID,
		stockMaster.CAdminID,
		stockMaster.TsCreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert stock master: %w", err)
	}

	// Insert stock details
	if len(stockDetails) > 0 {
		batch := &pgx.Batch{}
		detailQuery := `
			INSERT INTO stock_detail (c_id, c_stock_id, c_product_id, d_price, i_qty, c_currency)
			VALUES ($1, $2, $3, $4, $5, $6)
		`

		for _, detail := range stockDetails {
			batch.Queue(detailQuery,
				detail.CID,
				detail.CStockID,
				detail.CProductID,
				detail.DPrice,
				detail.IQty,
				detail.CCurrency,
			)
		}

		br := tx.SendBatch(ctx, batch)

		// Execute all batched queries
		for i := 0; i < len(stockDetails); i++ {
			_, err := br.Exec()
			if err != nil {
				br.Close()
				return fmt.Errorf("failed to insert stock detail at index %d: %w", i, err)
			}
		}
		br.Close()
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	slog.Info("Stock transaction completed successfully",
		"stock_id", stockMaster.CID,
		"merchant_id", stockMaster.CMerchantID,
		"detail_count", len(stockDetails))

	return nil
}

// ReduceStockByProductID reduces stock quantity for a specific product and returns the stock_detail ID
func (r *StockRepository) ReduceStockByProductID(ctx context.Context, tx pgx.Tx, productID string, qty int32, stockDetailID string) (string, error) {
	query := `
		UPDATE stock_detail
		SET i_qty = i_qty - $1
		WHERE c_product_id = $2 AND i_qty >= $1 AND c_id = $3
		RETURNING c_id
	`

	var updatedStockDetailID string
	err := tx.QueryRow(ctx, query, qty, productID, stockDetailID).Scan(&updatedStockDetailID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", fmt.Errorf("insufficient stock for product %s (required: %d)", productID, qty)
		}
		return "", fmt.Errorf("failed to reduce stock for product %s: %w", productID, err)
	}

	slog.Info("Reduced stock", "product_id", productID, "qty", qty, "stock_detail_id", updatedStockDetailID)
	return updatedStockDetailID, nil
}

// InsertSalesMaster inserts a new sales master record
func (r *StockRepository) InsertSalesMaster(ctx context.Context, tx pgx.Tx, salesMaster *models.SalesMaster) error {
	query := `
		INSERT INTO sales_master (c_id, ts_created_at, c_created_by, c_merchant_id)
		VALUES ($1, $2, $3, $4)
	`

	_, err := tx.Exec(ctx, query,
		salesMaster.CID,
		salesMaster.TsCreatedAt,
		salesMaster.CCreatedBy,
		salesMaster.CMerchantID,
	)

	if err != nil {
		return fmt.Errorf("failed to insert sales master: %w", err)
	}

	slog.Info("Inserted sales master", "sales_id", salesMaster.CID, "merchant_id", salesMaster.CMerchantID)
	return nil
}

// InsertSalesDetails inserts multiple sales detail records
func (r *StockRepository) InsertSalesDetails(ctx context.Context, tx pgx.Tx, salesDetails []models.SalesDetail) error {
	if len(salesDetails) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	query := `
		INSERT INTO sales_detail (c_id, c_sales_id, c_product_id, i_qty, d_price, c_currency, c_stock_detail_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	for _, detail := range salesDetails {
		batch.Queue(query,
			detail.CID,
			detail.CSalesID,
			detail.CProductID,
			detail.IQty,
			detail.DPrice,
			detail.CCurrency,
			detail.CStockDetailID,
		)
	}

	br := tx.SendBatch(ctx, batch)

	// Execute all batched queries
	for i := 0; i < len(salesDetails); i++ {
		_, err := br.Exec()
		if err != nil {
			br.Close()
			return fmt.Errorf("failed to insert sales detail at index %d: %w", i, err)
		}
	}
	br.Close()

	slog.Info("Inserted sales details", "count", len(salesDetails))
	return nil
}

// ProcessSalesTransaction processes a sales transaction: reduces stock and inserts sales records
func (r *StockRepository) ProcessSalesTransaction(ctx context.Context, salesMaster *models.SalesMaster, salesDetails []models.SalesDetail) error {
	// Begin transaction
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) // Rollback if not committed

	// Reduce stock for each product and update stock ID in sales details
	for i := range salesDetails {
		stockDetailID, err := r.ReduceStockByProductID(ctx, tx, salesDetails[i].CProductID, salesDetails[i].IQty, salesDetails[i].CStockDetailID)
		if err != nil {
			return err
		}
		// Update the CStockID with the actual stock_detail c_id
		salesDetails[i].CStockDetailID = stockDetailID
	}

	// Insert sales master
	if err := r.InsertSalesMaster(ctx, tx, salesMaster); err != nil {
		return err
	}

	// Insert sales details
	if err := r.InsertSalesDetails(ctx, tx, salesDetails); err != nil {
		return err
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	slog.Info("Sales transaction completed successfully",
		"sales_id", salesMaster.CID,
		"merchant_id", salesMaster.CMerchantID,
		"detail_count", len(salesDetails))

	return nil
}

// SalesHistoryDetail represents aggregated sales data by product
type SalesHistoryDetail struct {
	ProductID     string
	TotalQuantity int32
	MerchantID    string
	CreatedBy     string
}

// SalesGroupedDetail represents aggregated sales data by product and minute bucket
type SalesGroupedDetail struct {
	ProductID     string
	TotalQuantity int32
	MerchantID    string
	CreatedBy     string
	MinuteBucket  string
}

// GetSalesHistory retrieves sales history aggregated by product for a given time period
func (r *StockRepository) GetSalesHistory(ctx context.Context, merchantID string, timePeriod string) ([]SalesHistoryDetail, error) {
	var query string

	switch timePeriod {
	case "today":
		query = `
			with CTE_SALES_MASTER as (
				select t.c_id, t.c_merchant_id, t.ts_created_at, t.c_created_by 
				from sales_master t 
				where t.c_merchant_id = $1 AND DATE(t.ts_created_at) = CURRENT_DATE
			)
			select  
				A.c_product_id, 
				sum(A.i_qty) as total_quantity,
				max(B.c_merchant_id) as c_merchant_id, 
				max(B.c_created_by) as c_created_by
			from sales_detail A
			inner join CTE_SALES_MASTER B on B.c_id = A.c_sales_id 
			where A.c_sales_id in (select A1.c_id from CTE_SALES_MASTER A1)
			group by A.c_product_id
		`
	case "week":
		query = `
			with CTE_SALES_MASTER as (
				select t.c_id, t.c_merchant_id, t.ts_created_at, t.c_created_by 
				from sales_master t 
				where t.c_merchant_id = $1 
				AND t.ts_created_at >= CURRENT_DATE - INTERVAL '7 days'
			)
			select  
				A.c_product_id, 
				sum(A.i_qty) as total_quantity,
				B.c_merchant_id, 
				B.c_created_by
			from sales_detail A
			inner join CTE_SALES_MASTER B on B.c_id = A.c_sales_id 
			where A.c_sales_id in (select A1.c_id from CTE_SALES_MASTER A1)
			group by A.c_product_id, B.c_merchant_id, B.c_created_by
		`
	case "month":
		query = `
			with CTE_SALES_MASTER as (
				select t.c_id, t.c_merchant_id, t.ts_created_at, t.c_created_by 
				from sales_master t 
				where t.c_merchant_id = $1 
				AND DATE_TRUNC('month', t.ts_created_at) = DATE_TRUNC('month', CURRENT_DATE)
			)
			select  
				A.c_product_id, 
				sum(A.i_qty) as total_quantity,
				B.c_merchant_id, 
				B.c_created_by
			from sales_detail A
			inner join CTE_SALES_MASTER B on B.c_id = A.c_sales_id 
			where A.c_sales_id in (select A1.c_id from CTE_SALES_MASTER A1)
			group by A.c_product_id, B.c_merchant_id, B.c_created_by
		`
	default:
		return nil, fmt.Errorf("invalid time period: %s", timePeriod)
	}

	slog.Info("Executing sales history query", "merchant_id", merchantID, "time_period", timePeriod)

	rows, err := r.db.Query(ctx, query, merchantID)
	if err != nil {
		slog.Error("Failed to query sales history", "error", err, "merchant_id", merchantID, "time_period", timePeriod)
		return nil, fmt.Errorf("failed to query sales history: %w", err)
	}
	defer rows.Close()

	var salesHistory []SalesHistoryDetail
	for rows.Next() {
		var detail SalesHistoryDetail
		if err := rows.Scan(&detail.ProductID, &detail.TotalQuantity, &detail.MerchantID, &detail.CreatedBy); err != nil {
			slog.Error("Failed to scan sales history row", "error", err)
			return nil, fmt.Errorf("failed to scan sales history row: %w", err)
		}
		salesHistory = append(salesHistory, detail)
	}

	if err := rows.Err(); err != nil {
		slog.Error("Rows iteration error", "error", err)
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	slog.Info("Sales history query completed", "merchant_id", merchantID, "time_period", timePeriod, "row_count", len(salesHistory))
	return salesHistory, nil
}

// GetSalesHistoryTodayGrouped retrieves sales history for today grouped by product and minute bucket
func (r *StockRepository) GetSalesHistoryTodayGrouped(ctx context.Context, merchantID string) ([]SalesGroupedDetail, error) {
	query := `
		with CTE_SALES_MASTER as (
			select t.c_id, t.c_merchant_id , t.ts_created_at , t.c_created_by 
			from sales_master t 
			where t.c_merchant_id = $1 AND DATE(t.ts_created_at) = CURRENT_DATE
		)
		select  
			A.c_product_id, 
			sum(A.i_qty) as total_quantity,
			max(B.c_merchant_id) as c_merchant_id, 
			max(B.c_created_by) as c_created_by,
			date_trunc('minute', B.ts_created_at) AS minute_bucket
		from sales_detail A
		inner join CTE_SALES_MASTER B on B.c_id = A.c_sales_id 
		where A.c_sales_id in (select A1.c_id from CTE_SALES_MASTER A1)
		GROUP BY A.c_product_id, date_trunc('minute', B.ts_created_at)
	`

	slog.Info("Executing grouped sales history query", "merchant_id", merchantID)

	rows, err := r.db.Query(ctx, query, merchantID)
	if err != nil {
		slog.Error("Failed to query grouped sales history", "error", err, "merchant_id", merchantID)
		return nil, fmt.Errorf("failed to query grouped sales history: %w", err)
	}
	defer rows.Close()

	var results []SalesGroupedDetail
	// Get timezone from context
	loc := time.UTC
	if val := ctx.Value("timezone"); val != nil {
		if l, ok := val.(*time.Location); ok {
			loc = l
		}
	}
	slog.Info("Repository using timezone", "loc", loc.String())

	for rows.Next() {
		var detail SalesGroupedDetail
		var minuteBucket time.Time
		if err := rows.Scan(&detail.ProductID, &detail.TotalQuantity, &detail.MerchantID, &detail.CreatedBy, &minuteBucket); err != nil {
			slog.Error("Failed to scan grouped sales history row", "error", err)
			return nil, fmt.Errorf("failed to scan grouped sales history row: %w", err)
		}
		slog.Debug("Raw bucket from DB", "time", minuteBucket.String(), "product_id", detail.ProductID)
		detail.MinuteBucket = minuteBucket.In(loc).Format("15:04") // Format as hh:mm in requested timezone
		results = append(results, detail)
	}

	if err := rows.Err(); err != nil {
		slog.Error("Rows iteration error in grouped sales history", "error", err)
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	slog.Info("Grouped sales history query completed", "merchant_id", merchantID, "row_count", len(results))
	return results, nil
}

// InsertSalesDefectMaster inserts a new sales defect master record
func (r *StockRepository) InsertSalesDefectMaster(ctx context.Context, tx pgx.Tx, defectMaster *models.SalesDefectMaster) error {
	query := `
		INSERT INTO sales_defect_master (c_id, c_created_by, ts_created_at, c_merchant_id)
		VALUES ($1, $2, $3, $4)
	`

	_, err := tx.Exec(ctx, query,
		defectMaster.CID,
		defectMaster.CCreatedBy,
		defectMaster.TsCreatedAt,
		defectMaster.CMerchantID,
	)

	if err != nil {
		return fmt.Errorf("failed to insert sales defect master: %w", err)
	}

	slog.Info("Inserted sales defect master", "defect_id", defectMaster.CID, "merchant_id", defectMaster.CMerchantID)
	return nil
}

// InsertSalesDefectDetails inserts multiple sales defect detail records
func (r *StockRepository) InsertSalesDefectDetails(ctx context.Context, tx pgx.Tx, defectDetails []models.SalesDefectDetail) error {
	if len(defectDetails) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	query := `
		INSERT INTO sales_defect_detail (c_id, c_sales_defect_id, c_product_id, i_qty, d_price, c_currency, c_stock_detail_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	for _, detail := range defectDetails {
		batch.Queue(query,
			detail.CID,
			detail.CSalesDefectID,
			detail.CProductID,
			detail.IQty,
			detail.DPrice,
			detail.CCurrency,
			detail.CStockDetailID,
		)
	}

	br := tx.SendBatch(ctx, batch)

	// Execute all batched queries
	for i := 0; i < len(defectDetails); i++ {
		_, err := br.Exec()
		if err != nil {
			br.Close()
			return fmt.Errorf("failed to insert sales defect detail at index %d: %w", i, err)
		}
	}
	br.Close()

	slog.Info("Inserted sales defect details", "count", len(defectDetails))
	return nil
}

// ProcessDefectTransaction processes a defect transaction: reduces stock and inserts defect records
func (r *StockRepository) ProcessDefectTransaction(ctx context.Context, defectMaster *models.SalesDefectMaster, defectDetails []models.SalesDefectDetail) error {
	// Begin transaction
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) // Rollback if not committed

	// Reduce stock for each product and update stock ID in defect details
	for i := range defectDetails {
		stockDetailID, err := r.ReduceStockByProductID(ctx, tx, defectDetails[i].CProductID, defectDetails[i].IQty, defectDetails[i].CStockDetailID)
		if err != nil {
			return err
		}
		// Update the CStockDetailID with the actual stock_detail c_id (though it should already be correct, following ProcessSalesTransaction pattern)
		defectDetails[i].CStockDetailID = stockDetailID
	}

	// Insert sales defect master
	if err := r.InsertSalesDefectMaster(ctx, tx, defectMaster); err != nil {
		return err
	}

	// Insert sales defect details
	if err := r.InsertSalesDefectDetails(ctx, tx, defectDetails); err != nil {
		return err
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	slog.Info("Defect transaction completed successfully",
		"defect_id", defectMaster.CID,
		"merchant_id", defectMaster.CMerchantID,
		"detail_count", len(defectDetails))

	return nil
}

// SalesDefectAggregatedInfo represents aggregated sales defect data
type SalesDefectAggregatedInfo struct {
	ProductID     string
	Currency      string
	SubPrice      float64
	Qty           int32
	SubtotalPrice float64
}

// GetSalesDefectDetails retrieves aggregated sales defect details for today for a specific merchant
func (r *StockRepository) GetSalesDefectDetails(ctx context.Context, merchantID string) ([]SalesDefectAggregatedInfo, error) {
	query := `
		with CTE_SALES_DEFECT_MASTER as (
			select t.c_id, t.c_merchant_id , t.ts_created_at , t.c_created_by 
			from sales_defect_master t 
			where t.c_merchant_id = $1 AND DATE(t.ts_created_at) = CURRENT_DATE
		)
		select 
			sdd.c_product_id, 
			sdd.c_currency, 
			MAX(sdd.d_price) as sub_price, 
			SUM(sdd.i_qty) as total_qty,
			SUM(sdd.d_price * sdd.i_qty) as subtotal_price
		from sales_defect_detail sdd
		inner join CTE_SALES_DEFECT_MASTER A on A.c_id = sdd.c_sales_defect_id 
		where sdd.ts_deleted_at is null and sdd.c_deleted_by is null AND sdd.ts_created_at::date = CURRENT_DATE
		group by sdd.c_product_id, sdd.c_currency
	`

	slog.Info("Executing get aggregated sales defect details query", "merchant_id", merchantID)

	rows, err := r.db.Query(ctx, query, merchantID)
	if err != nil {
		slog.Error("Failed to query aggregated sales defect details", "error", err, "merchant_id", merchantID)
		return nil, fmt.Errorf("failed to query aggregated sales defect details: %w", err)
	}
	defer rows.Close()

	var results []SalesDefectAggregatedInfo
	for rows.Next() {
		var info SalesDefectAggregatedInfo
		err := rows.Scan(
			&info.ProductID,
			&info.Currency,
			&info.SubPrice,
			&info.Qty,
			&info.SubtotalPrice,
		)
		if err != nil {
			slog.Error("Failed to scan aggregated sales defect detail row", "error", err)
			return nil, fmt.Errorf("failed to scan aggregated sales defect detail row: %w", err)
		}
		results = append(results, info)
	}

	if err := rows.Err(); err != nil {
		slog.Error("Rows iteration error in get aggregated sales defect details", "error", err)
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	slog.Info("Get aggregated sales defect details query completed", "merchant_id", merchantID, "row_count", len(results))
	return results, nil
}
