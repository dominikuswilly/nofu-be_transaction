package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

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
