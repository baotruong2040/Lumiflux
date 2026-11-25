package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/truong/discord-clone-api/internal/domain"
	"github.com/truong/discord-clone-api/pkg/idgen"
)

type stubTicketStore struct {
	mu      sync.Mutex
	entries map[string]stubTicketEntry
}

type stubTicketEntry struct {
	userID uuid.UUID
	ttl    time.Duration
}

func newStubTicketStore() *stubTicketStore {
	return &stubTicketStore{
		entries: make(map[string]stubTicketEntry),
	}
}

func (s *stubTicketStore) SaveTicket(_ context.Context, ticket string, userID uuid.UUID, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[ticket] = stubTicketEntry{userID: userID, ttl: ttl}
	return nil
}

func (s *stubTicketStore) ConsumeTicket(_ context.Context, ticket string) (uuid.UUID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[ticket]
	if !ok {
		return uuid.Nil, domain.NewError(domain.ErrInvalidToken, "INVALID_TICKET", "websocket ticket is invalid or expired")
	}
	delete(s.entries, ticket)
	return entry.userID, nil
}

func TestTicketService_IssueAndSingleUseConsume(t *testing.T) {
	ctx := context.Background()
	store := newStubTicketStore()
	svc := NewTicketService(store, 30*time.Second)

	userID, err := idgen.NewUUIDv7()
	require.NoError(t, err)

	// Nil user rejected.
	_, err = svc.IssueTicket(ctx, uuid.Nil)
	assert.ErrorIs(t, err, domain.ErrUnauthorized)

	// Empty ticket rejected.
	_, err = svc.ConsumeTicket(ctx, "   ")
	assert.ErrorIs(t, err, domain.ErrInvalidToken)

	// Issue stores TTL 30s and returns non-empty opaque token.
	ticket, err := svc.IssueTicket(ctx, userID)
	require.NoError(t, err)
	assert.NotEmpty(t, ticket)
	assert.Equal(t, 30*time.Second, store.entries[ticket].ttl)

	// First consume succeeds and returns userID.
	gotUserID, err := svc.ConsumeTicket(ctx, ticket)
	require.NoError(t, err)
	assert.Equal(t, userID, gotUserID)

	// Replay fails immediately with ErrInvalidToken.
	_, err = svc.ConsumeTicket(ctx, ticket)
	assert.ErrorIs(t, err, domain.ErrInvalidToken)
}
