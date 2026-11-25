package redisstore

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// NewClient creates a configured go-redis client and verifies connectivity.
func NewClient(ctx context.Context, addr, password string, db int) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return client, nil
}

// HealthChecker adapts *redis.Client to domain.HealthChecker.
type HealthChecker struct {
	client *redis.Client
}

// NewHealthChecker creates a Redis health checker backed by client.
func NewHealthChecker(client *redis.Client) *HealthChecker {
	return &HealthChecker{client: client}
}

// Ping verifies that Redis responds within ctx.
func (h *HealthChecker) Ping(ctx context.Context) error {
	return h.client.Ping(ctx).Err()
}
