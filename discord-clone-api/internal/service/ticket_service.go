package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/truong/discord-clone-api/internal/domain"
)

// TicketService issues and atomically consumes single-use WebSocket upgrade tickets (ADR-004).
type TicketService struct {
	tickets domain.TicketStore
	ttl     time.Duration
}

// NewTicketService constructs a TicketService with the given store and ticket TTL.
func NewTicketService(tickets domain.TicketStore, ttl time.Duration) *TicketService {
	return &TicketService{
		tickets: tickets,
		ttl:     ttl,
	}
}

// IssueTicket creates a random opaque ticket bound to userID for WebSocket connection upgrade.
func (s *TicketService) IssueTicket(ctx context.Context, userID uuid.UUID) (string, error) {
	if userID == uuid.Nil {
		return "", domain.NewError(domain.ErrUnauthorized, "UNAUTHORIZED", "authentication required")
	}

	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate ws ticket: %w", err)
	}
	ticket := base64.RawURLEncoding.EncodeToString(buf)

	if err := s.tickets.SaveTicket(ctx, ticket, userID, s.ttl); err != nil {
		return "", err
	}
	return ticket, nil
}

// ConsumeTicket validates and atomically consumes a single-use WebSocket upgrade ticket.
func (s *TicketService) ConsumeTicket(ctx context.Context, rawTicket string) (uuid.UUID, error) {
	ticket := strings.TrimSpace(rawTicket)
	if ticket == "" {
		return uuid.Nil, domain.NewError(domain.ErrInvalidToken, "INVALID_TICKET", "missing websocket ticket")
	}
	return s.tickets.ConsumeTicket(ctx, ticket)
}
