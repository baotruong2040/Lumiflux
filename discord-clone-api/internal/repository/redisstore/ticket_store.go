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

// TicketStore implements domain.TicketStore using single-use Redis keys with GETDEL.
type TicketStore struct {
	client *redis.Client
}

// NewTicketStore constructs a TicketStore backed by client.
func NewTicketStore(client *redis.Client) *TicketStore {
	return &TicketStore{client: client}
}

func wsTicketKey(ticket string) string {
	return "wsticket:" + ticket
}

// SaveTicket stores a short-lived WebSocket upgrade ticket mapped to userID.
func (s *TicketStore) SaveTicket(ctx context.Context, ticket string, userID uuid.UUID, ttl time.Duration) error {
	if err := s.client.Set(ctx, wsTicketKey(ticket), userID.String(), ttl).Err(); err != nil {
		return fmt.Errorf("save ws ticket: %w", err)
	}
	return nil
}

// ConsumeTicket atomically retrieves and deletes a WebSocket upgrade ticket via GETDEL.
func (s *TicketStore) ConsumeTicket(ctx context.Context, ticket string) (uuid.UUID, error) {
	val, err := s.client.GetDel(ctx, wsTicketKey(ticket)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return uuid.Nil, domain.NewError(domain.ErrInvalidToken, "INVALID_TICKET", "websocket ticket is invalid or expired")
		}
		return uuid.Nil, fmt.Errorf("consume ws ticket: %w", err)
	}

	userID, err := uuid.Parse(val)
	if err != nil {
		return uuid.Nil, domain.NewError(domain.ErrInvalidToken, "INVALID_TICKET", "websocket ticket payload is invalid")
	}
	return userID, nil
}
