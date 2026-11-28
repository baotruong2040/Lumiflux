package redisstore

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/truong/discord-clone-api/internal/domain"
)

// TypingStore throttles TYPING_START events per (channel, user) using Redis SET NX EX (docs/architecture.md §4.2).
type TypingStore struct {
	client *redis.Client
}

var _ domain.TypingLimiter = (*TypingStore)(nil)

// NewTypingStore constructs a TypingStore backed by client.
func NewTypingStore(client *redis.Client) *TypingStore {
	return &TypingStore{client: client}
}

func typingKey(channelID, userID uuid.UUID) string {
	return "typing:" + channelID.String() + ":" + userID.String()
}

// AllowTyping returns true once per ttl window when SET NX succeeds for typing:{channelID}:{userID}.
func (s *TypingStore) AllowTyping(ctx context.Context, channelID, userID uuid.UUID, ttl time.Duration) (bool, error) {
	ok, err := s.client.SetNX(ctx, typingKey(channelID, userID), "1", ttl).Result()
	if err != nil {
		return false, fmt.Errorf("set typing key: %w", err)
	}
	return ok, nil
}
