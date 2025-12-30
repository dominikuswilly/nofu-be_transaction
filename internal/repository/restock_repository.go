package repository

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"nofu-be_transaction/internal/models"
)

type RestockRepository struct {
	db *pgxpool.Pool
}

func NewRestockRepository(db *pgxpool.Pool) *RestockRepository {
	return &RestockRepository{db: db}
}

// CreateRestock handles the creation of restock master, details, and history in a transaction
func (r *RestockRepository) CreateRestock(ctx context.Context, master *models.StockRestockMaster, details []models.StockRestockDetail, history *models.StockRestockHistory) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Insert into stock_restock_master
	masterQuery := `
		INSERT INTO stock_restock_master (c_id, c_merchant_id, c_status)
		VALUES ($1, $2, $3)
	`
	_, err = tx.Exec(ctx, masterQuery, master.CID, master.CMerchantID, master.CStatus)
	if err != nil {
		return fmt.Errorf("failed to insert restock master: %w", err)
	}

	// 2. Insert into stock_restock_detail
	if len(details) > 0 {
		batch := &pgx.Batch{}
		detailQuery := `
			INSERT INTO stock_restock_detail (c_id, c_stock_restock_id, c_product_id, i_qty)
			VALUES ($1, $2, $3, $4)
		`
		for _, d := range details {
			batch.Queue(detailQuery, d.CID, d.CStockRestockID, d.CProductID, d.IQty)
		}
		br := tx.SendBatch(ctx, batch)
		for i := 0; i < len(details); i++ {
			if _, err := br.Exec(); err != nil {
				br.Close()
				return fmt.Errorf("failed to insert restock detail at index %d: %w", i, err)
			}
		}
		br.Close()
	}

	// 3. Insert into stock_restock_history
	historyQuery := `
		INSERT INTO stock_restock_history (c_id, c_stock_restock_id, i_seq, c_status, c_created_by, ts_created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err = tx.Exec(ctx, historyQuery, history.CID, history.CStockRestockID, history.ISeq, history.CStatus, history.CCreatedBy, history.TsCreatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert restock history: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	slog.Info("Restock transaction committed successfully", "id", master.CID)
	return nil
}
