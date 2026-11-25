package redisstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/truong/discord-clone-api/internal/domain"
)

// TokenStore implements domain.TokenStore using Redis.
type TokenStore struct {
	client *redis.Client
}

// NewTokenStore creates a TokenStore backed by client.
func NewTokenStore(client *redis.Client) *TokenStore {
	return &TokenStore{client: client}
}

func refreshKey(hash string) string {
	return "refresh:" + hash
}

// SaveRefreshToken stores a refresh token hash mapped to userID with expiration ttl.
func (s *TokenStore) SaveRefreshToken(ctx context.Context, tokenHash string, userID uuid.UUID, ttl time.Duration) error {
	if err := s.client.Set(ctx, refreshKey(tokenHash), userID.String(), ttl).Err(); err != nil {
		return fmt.Errorf("save refresh token: %w", err)
	}
	return nil
}

// ConsumeRefreshToken atomically retrieves and deletes the refresh token entry via GETDEL.
func (s *TokenStore) ConsumeRefreshToken(ctx context.Context, tokenHash string) (uuid.UUID, error) {
	val, err := s.client.GetDel(ctx, refreshKey(tokenHash)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return uuid.Nil, domain.NewError(domain.ErrInvalidToken, "INVALID_REFRESH_TOKEN", "refresh token is invalid or expired")
		}
		return uuid.Nil, fmt.Errorf("consume refresh token: %w", err)
	}

	userID, err := uuid.Parse(val)
	if err != nil {
		return uuid.Nil, domain.NewError(domain.ErrInvalidToken, "INVALID_REFRESH_TOKEN", "refresh token payload is invalid")
	}
	return userID, nil
}

// DeleteRefreshToken revokes a refresh token hash from Redis.
func (s *TokenStore) DeleteRefreshToken(ctx context.Context, tokenHash string) error {
	if err := s.client.Del(ctx, refreshKey(tokenHash)).Err(); err != nil {
		return fmt.Errorf("delete refresh token: %w", err)
	}
	return nil
}
