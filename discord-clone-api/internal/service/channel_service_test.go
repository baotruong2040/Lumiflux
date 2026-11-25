package service

import (
	"context"
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

type stubChannelRepo struct {
	mu       sync.RWMutex
	channels map[uuid.UUID]domain.Channel
}

func newStubChannelRepo() *stubChannelRepo {
	return &stubChannelRepo{
		channels: make(map[uuid.UUID]domain.Channel),
	}
}

func (r *stubChannelRepo) Create(_ context.Context, channel *domain.Channel) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	maxPos := -1
	for _, existing := range r.channels {
		if existing.GuildID == channel.GuildID {
			if existing.Name == channel.Name {
				return domain.NewError(domain.ErrConflict, "CHANNEL_NAME_TAKEN", "channel name already exists in this guild")
			}
			if existing.Position > maxPos {
				maxPos = existing.Position
			}
		}
	}

	channel.Position = maxPos + 1
	channel.CreatedAt = time.Now().UTC()
	r.channels[channel.ID] = *channel
	return nil
}

func (r *stubChannelRepo) ListByGuild(_ context.Context, guildID uuid.UUID) ([]domain.Channel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]domain.Channel, 0)
	for _, ch := range r.channels {
		if ch.GuildID == guildID {
			out = append(out, ch)
		}
	}
	return out, nil
}

func (r *stubChannelRepo) GetByID(_ context.Context, channelID uuid.UUID) (*domain.Channel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ch, ok := r.channels[channelID]
	if !ok {
		return nil, domain.NewError(domain.ErrNotFound, "CHANNEL_NOT_FOUND", "channel not found")
	}
	cp := ch
	return &cp, nil
}

func (r *stubChannelRepo) Update(_ context.Context, channelID uuid.UUID, params domain.UpdateChannelParams) (*domain.Channel, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ch, ok := r.channels[channelID]
	if !ok {
		return nil, domain.NewError(domain.ErrNotFound, "CHANNEL_NOT_FOUND", "channel not found")
	}
	if params.Name != nil {
		for _, other := range r.channels {
			if other.GuildID == ch.GuildID && other.ID != ch.ID && other.Name == *params.Name {
				return nil, domain.NewError(domain.ErrConflict, "CHANNEL_NAME_TAKEN", "channel name already exists in this guild")
			}
		}
		ch.Name = *params.Name
	}
	if params.TopicSet {
		ch.Topic = params.Topic
	}
	r.channels[channelID] = ch
	cp := ch
	return &cp, nil
}

func (r *stubChannelRepo) Delete(_ context.Context, channelID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.channels[channelID]; !ok {
		return domain.NewError(domain.ErrNotFound, "CHANNEL_NOT_FOUND", "channel not found")
	}
	delete(r.channels, channelID)
	return nil
}

func setupChannelTestEnv(t *testing.T) (*ChannelService, *stubGuildRepo, *stubChannelRepo, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()

	guilds := newStubGuildRepo()
	channels := newStubChannelRepo()
	guildSvc := NewGuildService(guilds, nil, nil)
	channelSvc := NewChannelService(guilds, channels)

	ownerID, err := idgen.NewUUIDv7()
	require.NoError(t, err)
	adminID, err := idgen.NewUUIDv7()
	require.NoError(t, err)
	memberID, err := idgen.NewUUIDv7()
	require.NoError(t, err)
	outsiderID, err := idgen.NewUUIDv7()
	require.NoError(t, err)

	ctx := context.Background()
	guild, err := guildSvc.CreateGuild(ctx, ownerID, CreateGuildInput{Name: "Test Guild"})
	require.NoError(t, err)

	// Sync default channel from guild creation into stubChannelRepo and add admin & member roles.
	for _, ch := range guilds.channels[guild.ID] {
		channels.channels[ch.ID] = ch
	}
	guilds.members[guild.ID][adminID] = domain.GuildMember{
		GuildID: guild.ID,
		UserID:  adminID,
		Role:    domain.RoleAdmin,
	}
	guilds.members[guild.ID][memberID] = domain.GuildMember{
		GuildID: guild.ID,
		UserID:  memberID,
		Role:    domain.RoleMember,
	}

	return channelSvc, guilds, channels, guild.ID, ownerID, adminID, memberID, outsiderID
}

func TestChannelService_PermissionsAndList(t *testing.T) {
	ctx := context.Background()
	svc, _, _, guildID, ownerID, adminID, memberID, outsiderID := setupChannelTestEnv(t)

	// Member can list channels; outsider -> 403; missing guild -> 404.
	list, err := svc.ListChannels(ctx, memberID, guildID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "general", list[0].Name)

	_, err = svc.ListChannels(ctx, outsiderID, guildID)
	assert.ErrorIs(t, err, domain.ErrForbidden)

	missingID, _ := idgen.NewUUIDv7()
	_, err = svc.ListChannels(ctx, ownerID, missingID)
	assert.ErrorIs(t, err, domain.ErrNotFound)

	// Member & outsider cannot create channel -> 403.
	_, err = svc.CreateChannel(ctx, memberID, guildID, CreateChannelInput{Name: "member-ch"})
	assert.ErrorIs(t, err, domain.ErrForbidden)

	_, err = svc.CreateChannel(ctx, outsiderID, guildID, CreateChannelInput{Name: "outsider-ch"})
	assert.ErrorIs(t, err, domain.ErrForbidden)

	// Owner and Admin can create channels.
	topic := "Backend architecture discussion"
	ch1, err := svc.CreateChannel(ctx, ownerID, guildID, CreateChannelInput{Name: "  backend  ", Topic: &topic})
	require.NoError(t, err)
	assert.Equal(t, "backend", ch1.Name)
	require.NotNil(t, ch1.Topic)
	assert.Equal(t, topic, *ch1.Topic)
	assert.Equal(t, 1, ch1.Position)

	ch2, err := svc.CreateChannel(ctx, adminID, guildID, CreateChannelInput{Name: "frontend"})
	require.NoError(t, err)
	assert.Equal(t, 2, ch2.Position)

	// Member cannot update or delete -> 403.
	newName := "hacked"
	_, err = svc.UpdateChannel(ctx, memberID, ch1.ID, UpdateChannelInput{Name: &newName})
	assert.ErrorIs(t, err, domain.ErrForbidden)

	err = svc.DeleteChannel(ctx, memberID, ch1.ID)
	assert.ErrorIs(t, err, domain.ErrForbidden)

	// Admin updates and clears topic, then deletes.
	updatedTopic := "Core services"
	updated, err := svc.UpdateChannel(ctx, adminID, ch1.ID, UpdateChannelInput{
		Name:     &newName,
		Topic:    &updatedTopic,
		TopicSet: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "hacked", updated.Name)
	require.NotNil(t, updated.Topic)
	assert.Equal(t, "Core services", *updated.Topic)

	cleared, err := svc.UpdateChannel(ctx, ownerID, ch1.ID, UpdateChannelInput{
		Topic:    nil,
		TopicSet: true,
	})
	require.NoError(t, err)
	assert.Nil(t, cleared.Topic)

	require.NoError(t, svc.DeleteChannel(ctx, adminID, ch1.ID))
	err = svc.DeleteChannel(ctx, ownerID, ch1.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestChannelService_ValidationAndDuplicates(t *testing.T) {
	ctx := context.Background()
	svc, _, _, guildID, ownerID, _, _, _ := setupChannelTestEnv(t)

	// Empty / too-long name and too-long topic -> 400 validation errors.
	_, err := svc.CreateChannel(ctx, ownerID, guildID, CreateChannelInput{Name: "   "})
	assert.ErrorIs(t, err, domain.ErrValidation)

	_, err = svc.CreateChannel(ctx, ownerID, guildID, CreateChannelInput{Name: strings.Repeat("a", 101)})
	assert.ErrorIs(t, err, domain.ErrValidation)

	longTopic := strings.Repeat("t", 256)
	_, err = svc.CreateChannel(ctx, ownerID, guildID, CreateChannelInput{Name: "valid", Topic: &longTopic})
	assert.ErrorIs(t, err, domain.ErrValidation)

	// Duplicate channel name -> 409 Conflict.
	_, err = svc.CreateChannel(ctx, ownerID, guildID, CreateChannelInput{Name: "general"})
	assert.ErrorIs(t, err, domain.ErrConflict)

	ch, err := svc.CreateChannel(ctx, ownerID, guildID, CreateChannelInput{Name: "random"})
	require.NoError(t, err)

	dupName := "general"
	_, err = svc.UpdateChannel(ctx, ownerID, ch.ID, UpdateChannelInput{Name: &dupName})
	assert.ErrorIs(t, err, domain.ErrConflict)
}
