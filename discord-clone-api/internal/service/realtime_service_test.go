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

type stubPresenceStore struct {
	mu      sync.Mutex
	online  map[uuid.UUID]time.Duration
	removed []uuid.UUID
}

func newStubPresenceStore() *stubPresenceStore {
	return &stubPresenceStore{
		online: make(map[uuid.UUID]time.Duration),
	}
}

func (s *stubPresenceStore) GetOnlineMap(_ context.Context, userIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[uuid.UUID]bool, len(userIDs))
	for _, id := range userIDs {
		if _, ok := s.online[id]; ok {
			out[id] = true
		}
	}
	return out, nil
}

func (s *stubPresenceStore) SetOnline(_ context.Context, userID uuid.UUID, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.online[userID] = ttl
	return nil
}

func (s *stubPresenceStore) SetOffline(_ context.Context, userID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.online, userID)
	s.removed = append(s.removed, userID)
	return nil
}

type stubTypingLimiter struct {
	mu    sync.Mutex
	until map[string]time.Time
	now   func() time.Time
}

func newStubTypingLimiter(now func() time.Time) *stubTypingLimiter {
	return &stubTypingLimiter{
		until: make(map[string]time.Time),
		now:   now,
	}
}

func (l *stubTypingLimiter) AllowTyping(_ context.Context, channelID, userID uuid.UUID, ttl time.Duration) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := channelID.String() + ":" + userID.String()
	now := l.now()
	if exp, ok := l.until[key]; ok && now.Before(exp) {
		return false, nil
	}
	l.until[key] = now.Add(ttl)
	return true, nil
}

type presenceUpdateRecord struct {
	guildIDs []uuid.UUID
	userID   uuid.UUID
	online   bool
}

type userTypingRecord struct {
	guildID   uuid.UUID
	channelID uuid.UUID
	userID    uuid.UUID
	username  string
	timestamp int64
}

type recordingRealtimePublisher struct {
	mu        sync.Mutex
	presences []presenceUpdateRecord
	typings   []userTypingRecord
}

func (p *recordingRealtimePublisher) PublishPresenceUpdate(_ context.Context, guildIDs []uuid.UUID, userID uuid.UUID, online bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := append([]uuid.UUID(nil), guildIDs...)
	p.presences = append(p.presences, presenceUpdateRecord{guildIDs: cp, userID: userID, online: online})
}

func (p *recordingRealtimePublisher) PublishUserTyping(_ context.Context, guildID, channelID, userID uuid.UUID, username string, timestamp int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.typings = append(p.typings, userTypingRecord{
		guildID:   guildID,
		channelID: channelID,
		userID:    userID,
		username:  username,
		timestamp: timestamp,
	})
}

func TestRealtimeService_PresenceLifecycle(t *testing.T) {
	guilds := newStubGuildRepo()
	channels := newStubChannelRepo()
	presence := newStubPresenceStore()
	pub := &recordingRealtimePublisher{}

	svc := NewRealtimeService(guilds, channels, presence, nil, pub, pub, 60*time.Second, 3*time.Second)

	userID := idgen.MustUUIDv7()
	g1 := idgen.MustUUIDv7()
	g2 := idgen.MustUUIDv7()
	ctx := context.Background()

	svc.HandleUserOnline(ctx, userID, []uuid.UUID{g1, g2})
	assert.Equal(t, 60*time.Second, presence.online[userID])
	require.Len(t, pub.presences, 1)
	assert.True(t, pub.presences[0].online)
	assert.Equal(t, []uuid.UUID{g1, g2}, pub.presences[0].guildIDs)

	svc.HandleHeartbeat(ctx, userID)
	assert.Equal(t, 60*time.Second, presence.online[userID])

	svc.HandleUserOffline(ctx, userID, []uuid.UUID{g1, g2})
	_, stillOnline := presence.online[userID]
	assert.False(t, stillOnline)
	require.Len(t, pub.presences, 2)
	assert.False(t, pub.presences[1].online)
}

func TestRealtimeService_TypingRateLimitAndMembership(t *testing.T) {
	guilds := newStubGuildRepo()
	channels := newStubChannelRepo()
	currentNow := time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)
	nowFn := func() time.Time { return currentNow }
	typing := newStubTypingLimiter(nowFn)
	pub := &recordingRealtimePublisher{}

	ownerID := idgen.MustUUIDv7()
	outsiderID := idgen.MustUUIDv7()
	guilds.seedUser(ownerID, "typer_owner")
	guilds.seedUser(outsiderID, "outsider")

	guildSvc := NewGuildService(guilds, nil, nil, nil)
	guild, err := guildSvc.CreateGuild(context.Background(), ownerID, CreateGuildInput{Name: "Typing Guild"})
	require.NoError(t, err)

	var channelID uuid.UUID
	for _, ch := range guilds.channels[guild.ID] {
		channels.channels[ch.ID] = ch
		channelID = ch.ID
	}

	svc := NewRealtimeService(guilds, channels, nil, typing, pub, pub, 60*time.Second, 3*time.Second)
	svc.now = nowFn

	ctx := context.Background()
	// Outsider is rejected.
	err = svc.HandleTypingStart(ctx, outsiderID, channelID)
	assert.ErrorIs(t, err, domain.ErrForbidden)
	assert.Empty(t, pub.typings)

	// First typing allowed -> broadcasts USER_TYPING.
	require.NoError(t, svc.HandleTypingStart(ctx, ownerID, channelID))
	require.Len(t, pub.typings, 1)
	assert.Equal(t, "typer_owner", pub.typings[0].username)
	assert.Equal(t, guild.ID, pub.typings[0].guildID)

	// Immediate second typing within 3s -> suppressed cleanly without error.
	require.NoError(t, svc.HandleTypingStart(ctx, ownerID, channelID))
	assert.Len(t, pub.typings, 1)

	// After 3s -> allowed again.
	currentNow = currentNow.Add(3 * time.Second)
	require.NoError(t, svc.HandleTypingStart(ctx, ownerID, channelID))
	assert.Len(t, pub.typings, 2)
}
