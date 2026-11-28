package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/truong/discord-clone-api/internal/domain"
)

const (
	inviteCodeLength      = 8
	inviteAlphabet        = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	defaultInviteTTL      = 7 * 24 * time.Hour
	maxInviteUsesLimit    = 100000
	maxInviteExpireHours  = 8760
	inviteCreateMaxTries  = 5
)

// CreateInviteInput carries optional constraints when generating a guild invite.
type CreateInviteInput struct {
	MaxUses           *int
	ExpiresInHours    *int
	ExpiresInHoursSet bool
}

// InviteService implements invite creation and acceptance rules.
type InviteService struct {
	guilds        domain.GuildRepository
	invites       domain.InviteRepository
	subscriptions domain.GuildSubscriptionManager
	publisher     domain.GuildEventPublisher
	now           func() time.Time
}

// NewInviteService constructs an InviteService.
func NewInviteService(
	guilds domain.GuildRepository,
	invites domain.InviteRepository,
	subscriptions domain.GuildSubscriptionManager,
	publisher domain.GuildEventPublisher,
) *InviteService {
	if subscriptions == nil {
		subscriptions = domain.NoopGuildSubscriptionManager{}
	}
	if publisher == nil {
		publisher = domain.NoopGuildEventPublisher{}
	}
	return &InviteService{
		guilds:        guilds,
		invites:       invites,
		subscriptions: subscriptions,
		publisher:     publisher,
		now:           func() time.Time { return time.Now().UTC() },
	}
}

// CreateInvite creates a shareable invite code for guildID; any guild member is permitted (ADR-005).
func (s *InviteService) CreateInvite(ctx context.Context, userID, guildID uuid.UUID, input CreateInviteInput) (*domain.Invite, error) {
	if _, _, err := loadGuildAndMember(ctx, s.guilds, userID, guildID); err != nil {
		return nil, err
	}

	if input.MaxUses != nil && (*input.MaxUses < 1 || *input.MaxUses > maxInviteUsesLimit) {
		return nil, domain.NewError(domain.ErrValidation, "VALIDATION_ERROR", "maxUses must be between 1 and 100000")
	}

	var expiresAt *time.Time
	now := s.now()
	if !input.ExpiresInHoursSet {
		t := now.Add(defaultInviteTTL)
		expiresAt = &t
	} else if input.ExpiresInHours != nil {
		if *input.ExpiresInHours < 1 || *input.ExpiresInHours > maxInviteExpireHours {
			return nil, domain.NewError(domain.ErrValidation, "VALIDATION_ERROR", "expiresInHours must be between 1 and 8760")
		}
		t := now.Add(time.Duration(*input.ExpiresInHours) * time.Hour)
		expiresAt = &t
	}

	for range inviteCreateMaxTries {
		code, err := generateInviteCode(inviteCodeLength)
		if err != nil {
			return nil, fmt.Errorf("generate invite code: %w", err)
		}

		invite := &domain.Invite{
			Code:      code,
			GuildID:   guildID,
			CreatedBy: userID,
			MaxUses:   input.MaxUses,
			Uses:      0,
			ExpiresAt: expiresAt,
		}

		err = s.invites.Create(ctx, invite)
		if err == nil {
			return invite, nil
		}
		if !errors.Is(err, domain.ErrConflict) {
			return nil, err
		}
	}

	return nil, domain.NewError(domain.ErrInternal, "INTERNAL_ERROR", "failed to allocate a unique invite code")
}

// AcceptInvite validates and consumes an invite code, adding userID to the guild and subscribing sessions.
func (s *InviteService) AcceptInvite(ctx context.Context, userID uuid.UUID, code string) (*domain.Guild, error) {
	trimmed := strings.TrimSpace(code)
	if trimmed == "" || len(trimmed) > 12 {
		return nil, domain.NewError(domain.ErrValidation, "VALIDATION_ERROR", "invite code is invalid")
	}

	guild, member, err := s.invites.Accept(ctx, trimmed, userID, s.now())
	if err != nil {
		return nil, err
	}

	s.subscriptions.Subscribe(ctx, userID, guild.ID)
	if member != nil {
		s.publisher.PublishGuildMemberAdd(ctx, guild.ID, *member)
	}
	return guild, nil
}

func generateInviteCode(length int) (string, error) {
	maxIdx := big.NewInt(int64(len(inviteAlphabet)))
	out := make([]byte, length)
	for i := range out {
		n, err := rand.Int(rand.Reader, maxIdx)
		if err != nil {
			return "", err
		}
		out[i] = inviteAlphabet[n.Int64()]
	}
	return string(out), nil
}
