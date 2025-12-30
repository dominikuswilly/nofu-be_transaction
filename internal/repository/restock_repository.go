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
		INSERT INTO stock_restock_master (c_id, c_merchant_id, c_status, c_created_by, ts_created_at, d_longitude, d_latitude)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err = tx.Exec(ctx, masterQuery, master.CID, master.CMerchantID, master.CStatus, master.CCreatedBy, master.TsCreatedAt, master.DLongitude, master.DLatitude)
	if err != nil {
		return fmt.Errorf("failed to insert restock master: %w", err)
	}

	// 2. Insert into stock_restock_detail
	if len(details) > 0 {
		batch := &pgx.Batch{}
		detailQuery := `
			INSERT INTO stock_restock_detail (c_id, c_stock_restock_id, c_product_id, i_qty, c_created_by)
			VALUES ($1, $2, $3, $4, $5)
		`
		for _, d := range details {
			batch.Queue(detailQuery, d.CID, d.CStockRestockID, d.CProductID, d.IQty, d.CCreatedBy)
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

// GetRestockByMerchantID retrieves restock requests for a merchant from stock_restock_master
func (r *RestockRepository) GetRestockByMerchantID(ctx context.Context, merchantID string) ([]models.StockRestockMaster, error) {
	query := `
		SELECT 
			c_id, c_merchant_id, c_status, c_created_by, ts_created_at, 
			COALESCE(c_updated_by, '') as c_updated_by, 
			ts_updated_at,
			COALESCE(d_longitude, 0) as d_longitude,
			COALESCE(d_latitude, 0) as d_latitude
		FROM stock_restock_master
		WHERE c_merchant_id = $1
		ORDER BY ts_created_at DESC
	`

	rows, err := r.db.Query(ctx, query, merchantID)
	if err != nil {
		return nil, fmt.Errorf("failed to query restock master: %w", err)
	}
	defer rows.Close()

	var results []models.StockRestockMaster
	for rows.Next() {
		var m models.StockRestockMaster
		err := rows.Scan(
			&m.CID,
			&m.CMerchantID,
			&m.CStatus,
			&m.CCreatedBy,
			&m.TsCreatedAt,
			&m.CUpdatedBy,
			&m.TsUpdatedAt,
			&m.DLongitude,
			&m.DLatitude,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan restock master: %w", err)
		}
		results = append(results, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return results, nil
}

// GetRestockDetail retrieves restock details for a specific restock request
func (r *RestockRepository) GetRestockDetail(ctx context.Context, restockID string) ([]models.StockRestockDetail, error) {
	query := `
		SELECT c_id, c_stock_restock_id, c_product_id, i_qty, c_created_by
		FROM stock_restock_detail
		WHERE c_stock_restock_id = $1
	`

	rows, err := r.db.Query(ctx, query, restockID)
	if err != nil {
		return nil, fmt.Errorf("failed to query restock detail: %w", err)
	}
	defer rows.Close()

	var results []models.StockRestockDetail
	for rows.Next() {
		var d models.StockRestockDetail
		err := rows.Scan(
			&d.CID,
			&d.CStockRestockID,
			&d.CProductID,
			&d.IQty,
			&d.CCreatedBy,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan restock detail: %w", err)
		}
		results = append(results, d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return results, nil
}

// GetRestockHistory retrieves restock history for a specific restock request
func (r *RestockRepository) GetRestockHistory(ctx context.Context, restockID string) ([]models.StockRestockHistory, error) {
	query := `
		SELECT c_id, c_stock_restock_id, i_seq, c_status, c_created_by, ts_created_at
		FROM stock_restock_history
		WHERE c_stock_restock_id = $1
		ORDER BY i_seq ASC
	`

	rows, err := r.db.Query(ctx, query, restockID)
	if err != nil {
		return nil, fmt.Errorf("failed to query restock history: %w", err)
	}
	defer rows.Close()

	var results []models.StockRestockHistory
	for rows.Next() {
		var h models.StockRestockHistory
		err := rows.Scan(
			&h.CID,
			&h.CStockRestockID,
			&h.ISeq,
			&h.CStatus,
			&h.CCreatedBy,
			&h.TsCreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan restock history: %w", err)
		}
		results = append(results, h)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return results, nil
}
