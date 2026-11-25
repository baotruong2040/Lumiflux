package redisstore

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var allowScript = redis.NewScript(`
local current = redis.call("INCR", KEYS[1])
if current == 1 then
    redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
local pttl = redis.call("PTTL", KEYS[1])
return {current, pttl}
`)

// RateLimiter implements domain.RateLimiter using an atomic Redis Lua script.
type RateLimiter struct {
	client *redis.Client
}

// NewRateLimiter creates a Redis-backed RateLimiter.
func NewRateLimiter(client *redis.Client) *RateLimiter {
	return &RateLimiter{client: client}
}

// Allow increments the counter for key and checks whether it remains within limit over window.
func (r *RateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	windowMs := window.Milliseconds()
	if windowMs <= 0 {
		windowMs = 1000
	}

	res, err := allowScript.Run(ctx, r.client, []string{key}, windowMs).Int64Slice()
	if err != nil {
		return false, 0, fmt.Errorf("rate limit script: %w", err)
	}
	if len(res) < 2 {
		return false, 0, fmt.Errorf("unexpected rate limit script result length: %d", len(res))
	}

	current := res[0]
	pttl := res[1]

	if current <= int64(limit) {
		return true, 0, nil
	}

	retryAfter := time.Duration(pttl) * time.Millisecond
	if pttl <= 0 {
		retryAfter = window
	}
	return false, retryAfter, nil
}
