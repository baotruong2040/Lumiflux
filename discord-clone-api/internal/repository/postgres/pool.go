package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool creates a configured pgxpool.Pool and verifies database connectivity.
func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}

	cfg.MaxConns = 20
	cfg.MinConns = 2
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return pool, nil
}

// PoolHealthChecker adapts *pgxpool.Pool to domain.HealthChecker.
type PoolHealthChecker struct {
	pool *pgxpool.Pool
}

// NewHealthChecker creates a PostgreSQL health checker backed by pool.
func NewHealthChecker(pool *pgxpool.Pool) *PoolHealthChecker {
	return &PoolHealthChecker{pool: pool}
}

// Ping verifies that PostgreSQL responds within ctx.
func (h *PoolHealthChecker) Ping(ctx context.Context) error {
	return h.pool.Ping(ctx)
}
