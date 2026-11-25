package service

import (
	"bytes"
	"context"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/truong/discord-clone-api/internal/domain"
	"github.com/truong/discord-clone-api/pkg/idgen"
)

type stubMessageRepo struct {
	mu       sync.RWMutex
	messages map[uuid.UUID]domain.Message
	users    map[uuid.UUID]domain.User
}

func newStubMessageRepo(users map[uuid.UUID]domain.User) *stubMessageRepo {
	return &stubMessageRepo{
		messages: make(map[uuid.UUID]domain.Message),
		users:    users,
	}
}

func (r *stubMessageRepo) Create(_ context.Context, msg *domain.Message) (*domain.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	u := r.users[msg.AuthorID]
	stored := *msg
	stored.CreatedAt = time.Now().UTC()
	stored.Author = domain.MessageAuthor{
		ID:          msg.AuthorID,
		Username:    u.Username,
		DisplayName: u.DisplayName,
		AvatarURL:   u.AvatarURL,
	}
	r.messages[stored.ID] = stored
	cp := stored
	return &cp, nil
}

func (r *stubMessageRepo) ListByChannel(_ context.Context, channelID uuid.UUID, before *uuid.UUID, limit int) ([]domain.Message, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]domain.Message, 0)
	for _, m := range r.messages {
		if m.ChannelID != channelID || m.DeletedAt != nil {
			continue
		}
		if before != nil && bytes.Compare(m.ID[:], (*before)[:]) >= 0 {
			continue
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		return bytes.Compare(out[i].ID[:], out[j].ID[:]) > 0
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *stubMessageRepo) GetByID(_ context.Context, messageID uuid.UUID) (*domain.Message, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	m, ok := r.messages[messageID]
	if !ok || m.DeletedAt != nil {
		return nil, domain.NewError(domain.ErrNotFound, "MESSAGE_NOT_FOUND", "message not found")
	}
	cp := m
	return &cp, nil
}

func (r *stubMessageRepo) UpdateContent(_ context.Context, messageID uuid.UUID, content string) (*domain.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	m, ok := r.messages[messageID]
	if !ok || m.DeletedAt != nil {
		return nil, domain.NewError(domain.ErrNotFound, "MESSAGE_NOT_FOUND", "message not found")
	}
	now := time.Now().UTC()
	m.Content = content
	m.EditedAt = &now
	r.messages[messageID] = m
	cp := m
	return &cp, nil
}

func (r *stubMessageRepo) SoftDelete(_ context.Context, messageID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	m, ok := r.messages[messageID]
	if !ok || m.DeletedAt != nil {
		return domain.NewError(domain.ErrNotFound, "MESSAGE_NOT_FOUND", "message not found")
	}
	now := time.Now().UTC()
	m.DeletedAt = &now
	r.messages[messageID] = m
	return nil
}

type stubRateLimiter struct {
	mu     sync.Mutex
	counts map[string]int
}

func newStubRateLimiter() *stubRateLimiter {
	return &stubRateLimiter{counts: make(map[string]int)}
}

func (l *stubRateLimiter) Allow(_ context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.counts[key]++
	if l.counts[key] > limit {
		return false, window, nil
	}
	return true, 0, nil
}

type recordingMessagePublisher struct {
	mu      sync.Mutex
	created []domain.Message
	updated []domain.Message
	deleted []uuid.UUID
}

func (p *recordingMessagePublisher) PublishMessageCreate(_ context.Context, _ uuid.UUID, msg domain.Message) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.created = append(p.created, msg)
}

func (p *recordingMessagePublisher) PublishMessageUpdate(_ context.Context, _ uuid.UUID, msg domain.Message) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.updated = append(p.updated, msg)
}

func (p *recordingMessagePublisher) PublishMessageDelete(_ context.Context, _, _, messageID uuid.UUID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deleted = append(p.deleted, messageID)
}

func setupMessageTestEnv(t *testing.T) (
	*MessageService,
	*recordingMessagePublisher,
	uuid.UUID,
	uuid.UUID,
	uuid.UUID,
	uuid.UUID,
	uuid.UUID,
) {
	t.Helper()

	guilds := newStubGuildRepo()
	channels := newStubChannelRepo()
	guildSvc := NewGuildService(guilds, nil, nil)

	ownerID, _ := idgen.NewUUIDv7()
	adminID, _ := idgen.NewUUIDv7()
	memberID, _ := idgen.NewUUIDv7()
	outsiderID, _ := idgen.NewUUIDv7()

	guilds.seedUser(ownerID, "msg_owner")
	guilds.seedUser(adminID, "msg_admin")
	guilds.seedUser(memberID, "msg_member")
	guilds.seedUser(outsiderID, "msg_outsider")

	ctx := context.Background()
	guild, err := guildSvc.CreateGuild(ctx, ownerID, CreateGuildInput{Name: "Msg Guild"})
	require.NoError(t, err)

	var channelID uuid.UUID
	for _, ch := range guilds.channels[guild.ID] {
		channels.channels[ch.ID] = ch
		channelID = ch.ID
	}
	guilds.members[guild.ID][adminID] = domain.GuildMember{GuildID: guild.ID, UserID: adminID, Role: domain.RoleAdmin}
	guilds.members[guild.ID][memberID] = domain.GuildMember{GuildID: guild.ID, UserID: memberID, Role: domain.RoleMember}

	msgRepo := newStubMessageRepo(guilds.users)
	pub := &recordingMessagePublisher{}
	svc := NewMessageService(guilds, channels, msgRepo, newStubRateLimiter(), pub, 30, 10*time.Second)

	return svc, pub, channelID, ownerID, adminID, memberID, outsiderID
}

func TestMessageService_SendValidationAndRateLimit(t *testing.T) {
	ctx := context.Background()
	svc, pub, channelID, _, _, memberID, outsiderID := setupMessageTestEnv(t)

	// Outsider blocked -> 403
	_, err := svc.SendMessage(ctx, outsiderID, channelID, SendMessageInput{Content: "hello"})
	assert.ErrorIs(t, err, domain.ErrForbidden)

	// Empty / whitespace / >2000 runes / >64 nonce -> 400
	_, err = svc.SendMessage(ctx, memberID, channelID, SendMessageInput{Content: "   \n\t  "})
	assert.ErrorIs(t, err, domain.ErrValidation)

	_, err = svc.SendMessage(ctx, memberID, channelID, SendMessageInput{Content: strings.Repeat("ă", 2001)})
	assert.ErrorIs(t, err, domain.ErrValidation)

	longNonce := strings.Repeat("n", 65)
	_, err = svc.SendMessage(ctx, memberID, channelID, SendMessageInput{Content: "ok", Nonce: &longNonce})
	assert.ErrorIs(t, err, domain.ErrValidation)

	// 30 valid messages pass; 31st hits rate limit -> ErrRateLimited
	nonce := "nonce-1"
	first, err := svc.SendMessage(ctx, memberID, channelID, SendMessageInput{
		Content: "  Xin chào tiếng Việt  ",
		Nonce:   &nonce,
	})
	require.NoError(t, err)
	assert.Equal(t, "Xin chào tiếng Việt", first.Content)
	require.NotNil(t, first.Nonce)
	assert.Equal(t, "nonce-1", *first.Nonce)
	assert.Equal(t, "msg_member", first.Author.Username)

	for range 29 {
		_, err = svc.SendMessage(ctx, memberID, channelID, SendMessageInput{Content: strings.Repeat("x", 2000)})
		require.NoError(t, err)
	}

	_, err = svc.SendMessage(ctx, memberID, channelID, SendMessageInput{Content: "31st"})
	assert.ErrorIs(t, err, domain.ErrRateLimited)
	assert.Len(t, pub.created, 30)
}

func TestMessageService_ListEditAndDeleteRBAC(t *testing.T) {
	ctx := context.Background()
	svc, pub, channelID, ownerID, adminID, memberID, outsiderID := setupMessageTestEnv(t)

	// Member sends two messages, owner sends one.
	m1, err := svc.SendMessage(ctx, memberID, channelID, SendMessageInput{Content: "msg 1"})
	require.NoError(t, err)
	m2, err := svc.SendMessage(ctx, memberID, channelID, SendMessageInput{Content: "msg 2"})
	require.NoError(t, err)
	m3, err := svc.SendMessage(ctx, ownerID, channelID, SendMessageInput{Content: "msg 3"})
	require.NoError(t, err)

	// List pagination & validation
	_, err = svc.ListMessages(ctx, outsiderID, channelID, ListMessagesInput{})
	assert.ErrorIs(t, err, domain.ErrForbidden)

	_, err = svc.ListMessages(ctx, memberID, channelID, ListMessagesInput{Limit: 101})
	assert.ErrorIs(t, err, domain.ErrValidation)

	page1, err := svc.ListMessages(ctx, memberID, channelID, ListMessagesInput{Limit: 2})
	require.NoError(t, err)
	require.Len(t, page1, 2)
	assert.Equal(t, m3.ID, page1[0].ID)
	assert.Equal(t, m2.ID, page1[1].ID)

	page2, err := svc.ListMessages(ctx, memberID, channelID, ListMessagesInput{Before: &page1[1].ID, Limit: 2})
	require.NoError(t, err)
	require.Len(t, page2, 1)
	assert.Equal(t, m1.ID, page2[0].ID)

	// Only author can edit
	_, err = svc.UpdateMessage(ctx, ownerID, channelID, m1.ID, UpdateMessageInput{Content: "owner edit"})
	assert.ErrorIs(t, err, domain.ErrForbidden)

	edited, err := svc.UpdateMessage(ctx, memberID, channelID, m1.ID, UpdateMessageInput{Content: "  edited content  "})
	require.NoError(t, err)
	assert.Equal(t, "edited content", edited.Content)
	require.NotNil(t, edited.EditedAt)

	// Regular member cannot delete owner's message; admin can soft-delete member's message
	err = svc.DeleteMessage(ctx, memberID, channelID, m3.ID)
	assert.ErrorIs(t, err, domain.ErrForbidden)

	require.NoError(t, svc.DeleteMessage(ctx, adminID, channelID, m2.ID))
	require.NoError(t, svc.DeleteMessage(ctx, memberID, channelID, m1.ID))

	// Soft-deleted messages disappear from ListMessages and cannot be re-deleted (404)
	remaining, err := svc.ListMessages(ctx, memberID, channelID, ListMessagesInput{})
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	assert.Equal(t, m3.ID, remaining[0].ID)

	err = svc.DeleteMessage(ctx, ownerID, channelID, m2.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.Len(t, pub.updated, 1)
	assert.Len(t, pub.deleted, 2)
}
