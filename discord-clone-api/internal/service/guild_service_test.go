package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/truong/discord-clone-api/internal/domain"
	"github.com/truong/discord-clone-api/pkg/idgen"
)

type stubGuildRepo struct {
	mu       sync.RWMutex
	guilds   map[uuid.UUID]domain.Guild
	channels map[uuid.UUID][]domain.Channel
	members  map[uuid.UUID]map[uuid.UUID]domain.GuildMember
	users    map[uuid.UUID]domain.User
}

func newStubGuildRepo() *stubGuildRepo {
	return &stubGuildRepo{
		guilds:   make(map[uuid.UUID]domain.Guild),
		channels: make(map[uuid.UUID][]domain.Channel),
		members:  make(map[uuid.UUID]map[uuid.UUID]domain.GuildMember),
		users:    make(map[uuid.UUID]domain.User),
	}
}

func (r *stubGuildRepo) seedUser(id uuid.UUID, username string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.users[id] = domain.User{
		ID:          id,
		Username:    username,
		Email:       username + "@example.com",
		DisplayName: username,
		CreatedAt:   time.Now().UTC(),
	}
}

func (r *stubGuildRepo) CreateWithDefaultChannel(_ context.Context, guild *domain.Guild, ch *domain.Channel, owner *domain.GuildMember) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	guild.CreatedAt = now
	ch.CreatedAt = now
	owner.JoinedAt = now
	r.guilds[guild.ID] = *guild
	r.channels[guild.ID] = append(r.channels[guild.ID], *ch)
	if r.members[guild.ID] == nil {
		r.members[guild.ID] = make(map[uuid.UUID]domain.GuildMember)
	}
	r.members[guild.ID][owner.UserID] = *owner
	return nil
}

func (r *stubGuildRepo) ListByUser(_ context.Context, userID uuid.UUID) ([]domain.Guild, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Guild, 0)
	for gid, mems := range r.members {
		if _, ok := mems[userID]; ok {
			out = append(out, r.guilds[gid])
		}
	}
	return out, nil
}

func (r *stubGuildRepo) GetByID(_ context.Context, guildID uuid.UUID) (*domain.Guild, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	g, ok := r.guilds[guildID]
	if !ok {
		return nil, domain.NewError(domain.ErrNotFound, "GUILD_NOT_FOUND", "guild not found")
	}
	cp := g
	return &cp, nil
}

func (r *stubGuildRepo) Update(_ context.Context, guildID uuid.UUID, params domain.UpdateGuildParams) (*domain.Guild, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g, ok := r.guilds[guildID]
	if !ok {
		return nil, domain.NewError(domain.ErrNotFound, "GUILD_NOT_FOUND", "guild not found")
	}
	if params.Name != nil {
		g.Name = *params.Name
	}
	if params.IconURLSet {
		g.IconURL = params.IconURL
	}
	r.guilds[guildID] = g
	cp := g
	return &cp, nil
}

func (r *stubGuildRepo) Delete(_ context.Context, guildID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.guilds[guildID]; !ok {
		return domain.NewError(domain.ErrNotFound, "GUILD_NOT_FOUND", "guild not found")
	}
	delete(r.guilds, guildID)
	delete(r.channels, guildID)
	delete(r.members, guildID)
	return nil
}

func (r *stubGuildRepo) GetMember(_ context.Context, guildID, userID uuid.UUID) (*domain.GuildMember, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	mems := r.members[guildID]
	m, ok := mems[userID]
	if !ok {
		return nil, domain.NewError(domain.ErrNotFound, "MEMBER_NOT_FOUND", "member not found")
	}
	if u, hasUser := r.users[userID]; hasUser {
		m.User = u
	}
	cp := m
	return &cp, nil
}

func (r *stubGuildRepo) ListMembers(_ context.Context, guildID uuid.UUID) ([]domain.GuildMember, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	mems := r.members[guildID]
	out := make([]domain.GuildMember, 0, len(mems))
	for _, m := range mems {
		if u, ok := r.users[m.UserID]; ok {
			m.User = u
		}
		out = append(out, m)
	}
	return out, nil
}

func (r *stubGuildRepo) RemoveMember(_ context.Context, guildID, userID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	mems := r.members[guildID]
	if _, ok := mems[userID]; !ok {
		return domain.NewError(domain.ErrNotFound, "MEMBER_NOT_FOUND", "member not found")
	}
	delete(mems, userID)
	return nil
}

type stubPresenceReader struct {
	online map[uuid.UUID]bool
	err    error
}

func (p *stubPresenceReader) GetOnlineMap(_ context.Context, _ []uuid.UUID) (map[uuid.UUID]bool, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.online, nil
}

type subCall struct {
	userID  uuid.UUID
	guildID uuid.UUID
}

type spySubManager struct {
	mu           sync.Mutex
	subscribed   []subCall
	unsubscribed []subCall
}

func (s *spySubManager) Subscribe(_ context.Context, userID, guildID uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscribed = append(s.subscribed, subCall{userID: userID, guildID: guildID})
}

func (s *spySubManager) Unsubscribe(_ context.Context, userID, guildID uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unsubscribed = append(s.unsubscribed, subCall{userID: userID, guildID: guildID})
}

type recordingGuildPublisher struct {
	mu          sync.Mutex
	membersAdd  []domain.GuildMember
	membersLeft []struct {
		guildID uuid.UUID
		user    domain.User
	}
	deletedGuilds []uuid.UUID
}

func (p *recordingGuildPublisher) PublishGuildMemberAdd(_ context.Context, _ uuid.UUID, member domain.GuildMember) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.membersAdd = append(p.membersAdd, member)
}

func (p *recordingGuildPublisher) PublishGuildMemberRemove(_ context.Context, guildID uuid.UUID, user domain.User) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.membersLeft = append(p.membersLeft, struct {
		guildID uuid.UUID
		user    domain.User
	}{guildID: guildID, user: user})
}

func (p *recordingGuildPublisher) PublishGuildDelete(_ context.Context, guildID uuid.UUID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deletedGuilds = append(p.deletedGuilds, guildID)
}

func TestGuildService_CreateAndCRUD(t *testing.T) {
	repo := newStubGuildRepo()
	subs := &spySubManager{}
	pub := &recordingGuildPublisher{}
	ownerID := idgen.MustUUIDv7()
	otherID := idgen.MustUUIDv7()
	repo.seedUser(ownerID, "owner_user")
	repo.seedUser(otherID, "member_user")

	svc := NewGuildService(repo, &stubPresenceReader{online: map[uuid.UUID]bool{ownerID: true}}, subs, pub)

	// Invalid guild name
	_, err := svc.CreateGuild(context.Background(), ownerID, CreateGuildInput{Name: "   "})
	assert.ErrorIs(t, err, domain.ErrValidation)

	// Invalid icon URL
	badIcon := "ftp://bad.example.com/icon.png"
	_, err = svc.CreateGuild(context.Background(), ownerID, CreateGuildInput{Name: "Valid", IconURL: &badIcon})
	assert.ErrorIs(t, err, domain.ErrValidation)

	// Valid create -> creates guild + #general + owner + subscribes
	icon := "https://cdn.example.com/icon.png"
	guild, err := svc.CreateGuild(context.Background(), ownerID, CreateGuildInput{Name: "  Lumiflux HQ  ", IconURL: &icon})
	require.NoError(t, err)
	assert.Equal(t, "Lumiflux HQ", guild.Name)
	assert.Equal(t, ownerID, guild.OwnerID)
	require.Len(t, repo.channels[guild.ID], 1)
	assert.Equal(t, "general", repo.channels[guild.ID][0].Name)
	assert.Equal(t, int16(0), repo.channels[guild.ID][0].Type)
	assert.Equal(t, domain.RoleOwner, repo.members[guild.ID][ownerID].Role)
	assert.Equal(t, []subCall{{userID: ownerID, guildID: guild.ID}}, subs.subscribed)

	// Non-member cannot GetGuild
	_, err = svc.GetGuild(context.Background(), otherID, guild.ID)
	assert.ErrorIs(t, err, domain.ErrForbidden)

	// Missing guild returns ErrNotFound
	_, err = svc.GetGuild(context.Background(), ownerID, idgen.MustUUIDv7())
	assert.ErrorIs(t, err, domain.ErrNotFound)

	// Add second user as member; member cannot update or delete guild
	repo.members[guild.ID][otherID] = domain.GuildMember{GuildID: guild.ID, UserID: otherID, Role: domain.RoleMember}
	newName := "Hacked"
	_, err = svc.UpdateGuild(context.Background(), otherID, guild.ID, UpdateGuildInput{Name: &newName})
	assert.ErrorIs(t, err, domain.ErrForbidden)
	err = svc.DeleteGuild(context.Background(), otherID, guild.ID)
	assert.ErrorIs(t, err, domain.ErrForbidden)

	// Owner can update guild & clear iconUrl
	updatedName := "Lumiflux Renamed"
	empty := ""
	updated, err := svc.UpdateGuild(context.Background(), ownerID, guild.ID, UpdateGuildInput{
		Name:       &updatedName,
		IconURL:    &empty,
		IconURLSet: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "Lumiflux Renamed", updated.Name)
	assert.Nil(t, updated.IconURL)

	// ListMembers enriches online status
	members, err := svc.ListMembers(context.Background(), ownerID, guild.ID)
	require.NoError(t, err)
	assert.Len(t, members, 2)
	for _, m := range members {
		if m.UserID == ownerID {
			assert.True(t, m.Online)
		} else {
			assert.False(t, m.Online)
		}
	}

	// Owner can delete guild
	require.NoError(t, svc.DeleteGuild(context.Background(), ownerID, guild.ID))
	assert.Equal(t, []uuid.UUID{guild.ID}, pub.deletedGuilds)
	_, err = svc.GetGuild(context.Background(), ownerID, guild.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestGuildService_LeaveGuildAndADR007(t *testing.T) {
	repo := newStubGuildRepo()
	subs := &spySubManager{}
	pub := &recordingGuildPublisher{}
	ownerID := idgen.MustUUIDv7()
	memberID := idgen.MustUUIDv7()
	adminID := idgen.MustUUIDv7()
	repo.seedUser(ownerID, "owner")
	repo.seedUser(memberID, "member")
	repo.seedUser(adminID, "admin")

	svc := NewGuildService(repo, nil, subs, pub)
	guild, err := svc.CreateGuild(context.Background(), ownerID, CreateGuildInput{Name: "Leave Test"})
	require.NoError(t, err)

	repo.members[guild.ID][memberID] = domain.GuildMember{GuildID: guild.ID, UserID: memberID, Role: domain.RoleMember}
	repo.members[guild.ID][adminID] = domain.GuildMember{GuildID: guild.ID, UserID: adminID, Role: domain.RoleAdmin}

	// Owner cannot leave (ADR-006)
	err = svc.LeaveGuild(context.Background(), ownerID, guild.ID)
	assert.ErrorIs(t, err, domain.ErrForbidden)
	assert.Empty(t, subs.unsubscribed)
	assert.Empty(t, pub.membersLeft)

	// Member and Admin can leave and trigger hub.Unsubscribe (ADR-007)
	require.NoError(t, svc.LeaveGuild(context.Background(), memberID, guild.ID))
	require.NoError(t, svc.LeaveGuild(context.Background(), adminID, guild.ID))
	assert.Equal(t, []subCall{
		{userID: memberID, guildID: guild.ID},
		{userID: adminID, guildID: guild.ID},
	}, subs.unsubscribed)
	require.Len(t, pub.membersLeft, 2)
	assert.Equal(t, memberID, pub.membersLeft[0].user.ID)
	assert.Equal(t, "member", pub.membersLeft[0].user.Username)

	// Already left -> 403
	err = svc.LeaveGuild(context.Background(), memberID, guild.ID)
	assert.True(t, errors.Is(err, domain.ErrForbidden))
}
