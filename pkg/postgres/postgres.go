package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// cleanQuery removes newlines, tabs, and collapses multiple spaces into one
func cleanQuery(query string) string {
	// Replace newlines and tabs with spaces
	query = strings.ReplaceAll(query, "\n", " ")
	query = strings.ReplaceAll(query, "\t", " ")

	// Collapse multiple spaces into one
	for strings.Contains(query, "  ") {
		query = strings.ReplaceAll(query, "  ", " ")
	}

	// Trim leading and trailing spaces
	return strings.TrimSpace(query)
}

// queryTracer implements pgx.QueryTracer for logging SQL queries
type queryTracer struct{}

type queryTraceKey struct{}

type queryTraceData struct {
	sql       string
	args      []any
	startTime time.Time
}

func (qt *queryTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	traceData := &queryTraceData{
		sql:       data.SQL,
		args:      data.Args,
		startTime: time.Now(),
	}
	return context.WithValue(ctx, queryTraceKey{}, traceData)
}

func (qt *queryTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	traceData, ok := ctx.Value(queryTraceKey{}).(*queryTraceData)
	if !ok {
		return
	}

	duration := time.Since(traceData.startTime)

	if data.Err != nil {
		slog.Error("SQL query failed",
			"query", cleanQuery(traceData.sql),
			"args", traceData.args,
			"duration_ms", duration.Milliseconds(),
			"error", data.Err,
		)
	} else {
		slog.Info("SQL query executed",
			"query", cleanQuery(traceData.sql),
			"args", traceData.args,
			"duration_ms", duration.Milliseconds(),
			"rows_affected", data.CommandTag.RowsAffected(),
		)
	}
}

// NewClient creates a new PostgreSQL connection pool.
func NewClient(ctx context.Context, host, port, user, password, dbname, timezone string) (*pgxpool.Pool, error) {
	// Build connection string
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable timezone=%s",
		host, port, user, password, dbname, timezone)

	// Parse config for pool
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse pgxpool config: %w", err)
	}

	// Explicitly set timezone for ALL connections
	config.ConnConfig.RuntimeParams["timezone"] = timezone

	// Set connection pool settings
	config.MaxConns = 10
	config.MinConns = 2
	config.MaxConnLifetime = time.Hour

	// Attach query tracer for SQL logging
	config.ConnConfig.Tracer = &queryTracer{}

	// Create connection pool
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return pool, nil
}
