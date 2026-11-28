package redisstore

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/truong/discord-clone-api/internal/domain"
)

// PresenceStore manages user online presence flags in Redis (presence:{userID}).
type PresenceStore struct {
	client *redis.Client
}

var _ domain.PresenceStore = (*PresenceStore)(nil)

// NewPresenceStore constructs a PresenceStore backed by client.
func NewPresenceStore(client *redis.Client) *PresenceStore {
	return &PresenceStore{client: client}
}

func presenceKey(userID uuid.UUID) string {
	return "presence:" + userID.String()
}

// SetOnline marks userID as online in Redis with the configured TTL (60s default).
func (s *PresenceStore) SetOnline(ctx context.Context, userID uuid.UUID, ttl time.Duration) error {
	if err := s.client.Set(ctx, presenceKey(userID), "online", ttl).Err(); err != nil {
		return fmt.Errorf("set presence online: %w", err)
	}
	return nil
}

// SetOffline deletes userID's online presence key from Redis.
func (s *PresenceStore) SetOffline(ctx context.Context, userID uuid.UUID) error {
	if err := s.client.Del(ctx, presenceKey(userID)).Err(); err != nil {
		return fmt.Errorf("delete presence key: %w", err)
	}
	return nil
}

// GetOnlineMap returns a map of userID -> true for users with an active presence key in Redis.
func (s *PresenceStore) GetOnlineMap(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	result := make(map[uuid.UUID]bool, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}

	uniqueIDs := make([]uuid.UUID, 0, len(userIDs))
	keys := make([]string, 0, len(userIDs))
	seen := make(map[uuid.UUID]struct{}, len(userIDs))
	for _, id := range userIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
		keys = append(keys, presenceKey(id))
	}

	vals, err := s.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("mget presence keys: %w", err)
	}

	for i, val := range vals {
		if val != nil {
			result[uniqueIDs[i]] = true
		}
	}
	return result, nil
}
