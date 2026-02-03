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
		INSERT INTO stock_restock_master (c_id, c_merchant_id, c_merchant_nm, c_status, c_created_by, ts_created_at, d_longitude, d_latitude)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err = tx.Exec(ctx, masterQuery, master.CID, master.CMerchantID, master.CMerchantNm, master.CStatus, master.CCreatedBy, master.TsCreatedAt, master.DLongitude, master.DLatitude)
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
		WHERE c_merchant_id = $1 and ts_deleted_at is null AND c_deleted_by is null
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

// GetRestockByID retrieves a single restock request by its ID
func (r *RestockRepository) GetRestockByID(ctx context.Context, id string) (*models.StockRestockMaster, error) {
	query := `
		SELECT 
			c_id, c_merchant_id, c_status, c_created_by, ts_created_at, 
			COALESCE(c_updated_by, '') as c_updated_by, 
			ts_updated_at,
			COALESCE(d_longitude, 0) as d_longitude,
			COALESCE(d_latitude, 0) as d_latitude
		FROM stock_restock_master
		WHERE c_id = $1 AND ts_deleted_at is null AND c_deleted_by is null
	`

	var m models.StockRestockMaster
	err := r.db.QueryRow(ctx, query, id).Scan(
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
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query restock master by id: %w", err)
	}

	return &m, nil
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

// GetAllRestock retrieves all restock requests across all merchants, optionally filtered by date range
func (r *RestockRepository) GetAllRestock(ctx context.Context, timeStart, timeEnd string) ([]models.StockRestockMaster, error) {
	query := `
		SELECT 
			c_id, c_merchant_id, c_status, c_created_by, ts_created_at, 
			COALESCE(c_updated_by, '') as c_updated_by, 
			ts_updated_at,
			COALESCE(d_longitude, 0) as d_longitude,
			COALESCE(d_latitude, 0) as d_latitude
		FROM stock_restock_master
		WHERE ts_deleted_at IS NULL AND c_deleted_by IS NULL
	`

	args := []interface{}{}
	argIndex := 1

	// Add date range filters if provided
	if timeStart != "" && timeEnd != "" {
		query += fmt.Sprintf(" AND DATE(ts_created_at) BETWEEN $%d AND $%d", argIndex, argIndex+1)
		args = append(args, timeStart, timeEnd)
		argIndex += 2
	} else if timeStart != "" {
		query += fmt.Sprintf(" AND DATE(ts_created_at) >= $%d", argIndex)
		args = append(args, timeStart)
		argIndex++
	} else if timeEnd != "" {
		query += fmt.Sprintf(" AND DATE(ts_created_at) <= $%d", argIndex)
		args = append(args, timeEnd)
		argIndex++
	}

	query += " ORDER BY ts_created_at DESC"

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query restock master for admin: %w", err)
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
			return nil, fmt.Errorf("failed to scan restock master for admin: %w", err)
		}
		results = append(results, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error for admin: %w", err)
	}

	return results, nil
}

// ApproveRestock updates the status of a restock request to APPROVED within a transaction
func (r *RestockRepository) ApproveRestock(ctx context.Context, id, userID string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Validation Query
	validationQuery := `
		SELECT c_id 
		FROM stock_restock_master 
		WHERE c_id = $1
		  AND DATE(ts_created_at) = CURRENT_DATE 
		  AND ts_deleted_at IS NULL 
		  AND c_deleted_by IS NULL
		  AND c_status = 'PENDING'
		FOR UPDATE
	`
	var existingID string
	err = tx.QueryRow(ctx, validationQuery, id).Scan(&existingID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("restock not eligible for approval (not found, incorrect status, or already processed)")
		}
		return fmt.Errorf("validation query failed: %w", err)
	}

	// 2. Update status and updated_by/at (using userID as the approver/updater)
	updateQuery := `
		UPDATE stock_restock_master 
		SET c_status = 'APPROVED', c_updated_by = $2, ts_updated_at = NOW()
		WHERE c_id = $1
	`
	_, err = tx.Exec(ctx, updateQuery, id, userID)
	if err != nil {
		return fmt.Errorf("failed to update restock status: %w", err)
	}

	// 3. Insert history
	// Look up the last sequence number for this restock
	seqQuery := `SELECT COALESCE(MAX(i_seq), 0) + 1 FROM stock_restock_history WHERE c_stock_restock_id = $1`
	var nextSeq int
	err = tx.QueryRow(ctx, seqQuery, id).Scan(&nextSeq)
	if err != nil {
		return fmt.Errorf("failed to get next sequence: %w", err)
	}

	historyQuery := `
		INSERT INTO stock_restock_history (c_id, c_stock_restock_id, i_seq, c_status, c_created_by, ts_created_at)
		VALUES (gen_random_uuid()::text, $1, $2, 'APPROVED', $3, NOW())
	`
	_, err = tx.Exec(ctx, historyQuery, id, nextSeq, userID)
	if err != nil {
		return fmt.Errorf("failed to insert approval history: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// RejectRestock soft deletes a restock request within a transaction
func (r *RestockRepository) RejectRestock(ctx context.Context, id, userID string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Validation Query
	validationQuery := `
		SELECT c_id 
		FROM stock_restock_master 
		WHERE c_id = $1
		  AND DATE(ts_created_at) = CURRENT_DATE 
		  AND ts_deleted_at IS NULL 
		  AND c_deleted_by IS NULL
		  AND c_status = 'PENDING'
		FOR UPDATE
	`
	var existingID string
	err = tx.QueryRow(ctx, validationQuery, id).Scan(&existingID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("restock not eligible for rejection (not found, incorrect status, or already processed)")
		}
		return fmt.Errorf("validation query failed: %w", err)
	}

	// 2. Soft delete
	deleteQuery := `
		UPDATE stock_restock_master 
		SET ts_deleted_at = NOW(), c_deleted_by = $2, c_status='REJECTED'
		WHERE c_id = $1
	`
	_, err = tx.Exec(ctx, deleteQuery, id, userID)
	if err != nil {
		return fmt.Errorf("failed to delete restock: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetRestockHistoryByDate retrieves all restock requests for a specific date across all merchants
func (r *RestockRepository) GetRestockHistoryByDate(ctx context.Context, date string) ([]models.StockRestockMaster, error) {
	query := `
		SELECT 
			c_id, c_merchant_id, c_status, c_created_by, ts_created_at, 
			COALESCE(c_updated_by, '') as c_updated_by, 
			ts_updated_at,
			COALESCE(d_longitude, 0) as d_longitude,
			COALESCE(d_latitude, 0) as d_latitude
		FROM stock_restock_master
		WHERE ts_deleted_at IS NULL AND c_deleted_by IS NULL
		AND DATE(ts_created_at) = $1
		ORDER BY ts_created_at DESC
	`

	rows, err := r.db.Query(ctx, query, date)
	if err != nil {
		return nil, fmt.Errorf("failed to query restock history by date: %w", err)
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
			return nil, fmt.Errorf("failed to scan restock history by date: %w", err)
		}
		results = append(results, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error for history by date: %w", err)
	}

	return results, nil
}
