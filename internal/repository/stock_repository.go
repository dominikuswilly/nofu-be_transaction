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
