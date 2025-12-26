package repository

import (
	"context"
	"fmt"
	"log/slog"

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
func (r *StockRepository) ReduceStockByProductID(ctx context.Context, tx pgx.Tx, productID string, qty int32, stockID string) (string, error) {
	query := `
		UPDATE stock_detail
		SET i_qty = i_qty - $1
		WHERE c_product_id = $2 AND i_qty >= $1 AND c_stock_id = $3
		RETURNING c_id
	`

	var updatedStockDetailID string
	err := tx.QueryRow(ctx, query, qty, productID, stockID).Scan(&updatedStockDetailID)
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
		INSERT INTO sales_detail (c_id, c_sales_id, c_product_id, i_qty, d_price, c_currency, c_stock_id)
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
			detail.CStockID,
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
		_, err := r.ReduceStockByProductID(ctx, tx, salesDetails[i].CProductID, salesDetails[i].IQty, salesDetails[i].CStockID)
		if err != nil {
			return err
		}
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
