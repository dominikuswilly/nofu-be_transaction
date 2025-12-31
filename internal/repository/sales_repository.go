package repository

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type SalesRepository struct {
	db *pgxpool.Pool
}

func NewSalesRepository(db *pgxpool.Pool) *SalesRepository {
	return &SalesRepository{db: db}
}

// MerchantBalanceGroup represents the balance grouped by payment method
type MerchantBalanceGroup struct {
	PaymentMethod string  `json:"paymentMethod"`
	TotalBalance  float64 `json:"totalBalance"`
}

// MerchantBalanceResult represents the overall balance result for a merchant
type MerchantBalanceResult struct {
	TotalBalance float64                `json:"totalBalance"`
	BalanceGroup []MerchantBalanceGroup `json:"balanceGroup"`
}

// GetMerchantBalance retrieves the balance for a merchant grouped by payment method for today
func (r *SalesRepository) GetMerchantBalance(ctx context.Context, merchantID string) (*MerchantBalanceResult, error) {
	query := `
		with CTE_SALES_MASTER as (
			select sm.c_id, sm.c_merchant_id, ts_created_at, c_payment_method, d_total_payment 
			from sales_master sm 
			where sm.c_merchant_id = $1 
			  and sm.ts_created_at::date = current_date 
			  and sm.ts_deleted_at is null 
			  and sm.c_deleted_by is null
		)
		select c_payment_method, c_merchant_id, sum(d_total_payment) as total_balance 
		from CTE_SALES_MASTER
		group by c_payment_method, c_merchant_id
	`

	rows, err := r.db.Query(ctx, query, merchantID)
	if err != nil {
		return nil, fmt.Errorf("failed to query merchant balance: %w", err)
	}
	defer rows.Close()

	var totalBalance float64
	var balanceGroups []MerchantBalanceGroup

	for rows.Next() {
		var group MerchantBalanceGroup
		var mID string
		if err := rows.Scan(&group.PaymentMethod, &mID, &group.TotalBalance); err != nil {
			return nil, fmt.Errorf("failed to scan balance row: %w", err)
		}
		totalBalance += group.TotalBalance
		balanceGroups = append(balanceGroups, group)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	result := &MerchantBalanceResult{
		TotalBalance: totalBalance,
		BalanceGroup: balanceGroups,
	}

	return result, nil
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
func (r *SalesRepository) GetSalesHistory(ctx context.Context, merchantID string, timePeriod string) ([]SalesHistoryDetail, error) {
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
func (r *SalesRepository) GetSalesHistoryTodayGrouped(ctx context.Context, merchantID string) ([]SalesGroupedDetail, error) {
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
