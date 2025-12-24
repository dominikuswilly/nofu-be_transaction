package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

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
			"query", traceData.sql,
			"args", traceData.args,
			"duration_ms", duration.Milliseconds(),
			"error", data.Err,
		)
	} else {
		slog.Info("SQL query executed",
			"query", traceData.sql,
			"args", traceData.args,
			"duration_ms", duration.Milliseconds(),
			"rows_affected", data.CommandTag.RowsAffected(),
		)
	}
}

// NewClient creates a new PostgreSQL connection pool.
func NewClient(ctx context.Context, host, port, user, password, dbname string) (*pgxpool.Pool, error) {
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, password, host, port, dbname)

	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("unable to parse database config: %w", err)
	}

	// Set connection pool settings
	config.MaxConns = 10
	config.MinConns = 2
	config.MaxConnLifetime = time.Hour

	// Attach query tracer for SQL logging
	config.ConnConfig.Tracer = &queryTracer{}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection pool: %w", err)
	}

	// Ping the database to verify connection
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("unable to connect to database: %w", err)
	}

	return pool, nil
}
