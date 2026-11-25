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

type stubInviteRepo struct {
	mu      sync.Mutex
	guildDB *stubGuildRepo
	invites map[string]domain.Invite
}

func newStubInviteRepo(guildDB *stubGuildRepo) *stubInviteRepo {
	return &stubInviteRepo{
		guildDB: guildDB,
		invites: make(map[string]domain.Invite),
	}
}

func (r *stubInviteRepo) Create(_ context.Context, inv *domain.Invite) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.invites[inv.Code]; exists {
		return domain.NewError(domain.ErrConflict, "INVITE_CODE_CONFLICT", "invite code already exists")
	}
	inv.CreatedAt = time.Now().UTC()
	r.invites[inv.Code] = *inv
	return nil
}

func (r *stubInviteRepo) Accept(_ context.Context, code string, userID uuid.UUID, now time.Time) (*domain.Guild, *domain.GuildMember, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	inv, ok := r.invites[code]
	if !ok {
		return nil, nil, domain.NewError(domain.ErrNotFound, "INVITE_NOT_FOUND", "invite code not found")
	}
	if inv.ExpiresAt != nil && !inv.ExpiresAt.After(now) {
		return nil, nil, domain.NewError(domain.ErrValidation, "INVITE_EXPIRED", "invite code has expired")
	}
	if inv.MaxUses != nil && inv.Uses >= *inv.MaxUses {
		return nil, nil, domain.NewError(domain.ErrValidation, "INVITE_EXHAUSTED", "invite code has reached its maximum uses")
	}

	r.guildDB.mu.Lock()
	defer r.guildDB.mu.Unlock()
	if _, already := r.guildDB.members[inv.GuildID][userID]; already {
		return nil, nil, domain.NewError(domain.ErrConflict, "ALREADY_MEMBER", "you are already a member of this guild")
	}

	inv.Uses++
	r.invites[code] = inv

	member := domain.GuildMember{
		GuildID:  inv.GuildID,
		UserID:   userID,
		Role:     domain.RoleMember,
		JoinedAt: now,
		User:     r.guildDB.users[userID],
	}
	r.guildDB.members[inv.GuildID][userID] = member
	g := r.guildDB.guilds[inv.GuildID]
	return &g, &member, nil
}

func TestInviteService_CreateAndAcceptLifecycle(t *testing.T) {
	guildRepo := newStubGuildRepo()
	inviteRepo := newStubInviteRepo(guildRepo)
	subs := &spySubManager{}

	ownerID := idgen.MustUUIDv7()
	joinerID := idgen.MustUUIDv7()
	thirdID := idgen.MustUUIDv7()
	outsiderID := idgen.MustUUIDv7()
	guildRepo.seedUser(ownerID, "owner")
	guildRepo.seedUser(joinerID, "joiner")
	guildRepo.seedUser(thirdID, "third")
	guildRepo.seedUser(outsiderID, "outsider")

	fixedNow := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	guildSvc := NewGuildService(guildRepo, nil, subs)
	inviteSvc := NewInviteService(guildRepo, inviteRepo, subs)
	inviteSvc.now = func() time.Time { return fixedNow }

	guild, err := guildSvc.CreateGuild(context.Background(), ownerID, CreateGuildInput{Name: "Invite Guild"})
	require.NoError(t, err)

	// Non-member cannot create invite -> 403
	_, err = inviteSvc.CreateInvite(context.Background(), outsiderID, guild.ID, CreateInviteInput{})
	assert.ErrorIs(t, err, domain.ErrForbidden)

	// Validation checks on maxUses / expiresInHours
	zero := 0
	_, err = inviteSvc.CreateInvite(context.Background(), ownerID, guild.ID, CreateInviteInput{MaxUses: &zero})
	assert.ErrorIs(t, err, domain.ErrValidation)

	_, err = inviteSvc.CreateInvite(context.Background(), ownerID, guild.ID, CreateInviteInput{ExpiresInHours: &zero, ExpiresInHoursSet: true})
	assert.ErrorIs(t, err, domain.ErrValidation)

	// Default invite -> nil maxUses, expiresAt = +7d
	defaultInv, err := inviteSvc.CreateInvite(context.Background(), ownerID, guild.ID, CreateInviteInput{})
	require.NoError(t, err)
	assert.Len(t, defaultInv.Code, inviteCodeLength)
	assert.Nil(t, defaultInv.MaxUses)
	require.NotNil(t, defaultInv.ExpiresAt)
	assert.Equal(t, fixedNow.Add(7*24*time.Hour), *defaultInv.ExpiresAt)

	// Single-use invite
	one := 1
	singleInv, err := inviteSvc.CreateInvite(context.Background(), ownerID, guild.ID, CreateInviteInput{
		MaxUses:           &one,
		ExpiresInHours:    nil,
		ExpiresInHoursSet: true,
	})
	require.NoError(t, err)
	assert.Nil(t, singleInv.ExpiresAt)

	// Accept single-use invite succeeds & subscribes joiner
	joinedGuild, err := inviteSvc.AcceptInvite(context.Background(), joinerID, singleInv.Code)
	require.NoError(t, err)
	assert.Equal(t, guild.ID, joinedGuild.ID)
	assert.Contains(t, subs.subscribed, subCall{userID: joinerID, guildID: guild.ID})

	// Member (non-owner) can also create an invite (ADR-005)
	twoHours := 2
	memberInv, err := inviteSvc.CreateInvite(context.Background(), joinerID, guild.ID, CreateInviteInput{
		ExpiresInHours:    &twoHours,
		ExpiresInHoursSet: true,
	})
	require.NoError(t, err)

	// Duplicate accept by same user -> 409 ALREADY_MEMBER
	_, err = inviteSvc.AcceptInvite(context.Background(), joinerID, defaultInv.Code)
	assert.ErrorIs(t, err, domain.ErrConflict)

	// Exhausted invite -> 400 INVITE_EXHAUSTED
	_, err = inviteSvc.AcceptInvite(context.Background(), thirdID, singleInv.Code)
	var domErr *domain.Error
	require.ErrorAs(t, err, &domErr)
	assert.ErrorIs(t, err, domain.ErrValidation)
	assert.Equal(t, "INVITE_EXHAUSTED", domErr.Code)

	// Expired invite -> 400 INVITE_EXPIRED
	fixedNow = fixedNow.Add(3 * time.Hour)
	_, err = inviteSvc.AcceptInvite(context.Background(), thirdID, memberInv.Code)
	require.ErrorAs(t, err, &domErr)
	assert.ErrorIs(t, err, domain.ErrValidation)
	assert.Equal(t, "INVITE_EXPIRED", domErr.Code)

	// Missing invite -> 404 INVITE_NOT_FOUND
	_, err = inviteSvc.AcceptInvite(context.Background(), thirdID, "MISSING1")
	assert.ErrorIs(t, err, domain.ErrNotFound)
}
