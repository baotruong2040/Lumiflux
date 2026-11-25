package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/truong/discord-clone-api/internal/domain"
	"github.com/truong/discord-clone-api/pkg/idgen"
)

// CreateGuildInput carries validated fields for creating a guild.
type CreateGuildInput struct {
	Name    string
	IconURL *string
}

// UpdateGuildInput carries mutable guild fields for partial updates.
type UpdateGuildInput struct {
	Name       *string
	IconURL    *string
	IconURLSet bool
}

// GuildService implements guild management, membership, and leave workflows.
type GuildService struct {
	guilds        domain.GuildRepository
	presence      domain.PresenceReader
	subscriptions domain.GuildSubscriptionManager
}

// NewGuildService constructs a GuildService with optional presence and subscription hooks.
func NewGuildService(
	guilds domain.GuildRepository,
	presence domain.PresenceReader,
	subscriptions domain.GuildSubscriptionManager,
) *GuildService {
	if subscriptions == nil {
		subscriptions = domain.NoopGuildSubscriptionManager{}
	}
	return &GuildService{
		guilds:        guilds,
		presence:      presence,
		subscriptions: subscriptions,
	}
}

// CreateGuild creates a guild together with its default #general text channel and owner membership.
func (s *GuildService) CreateGuild(ctx context.Context, ownerID uuid.UUID, input CreateGuildInput) (*domain.Guild, error) {
	name, err := validateGuildName(input.Name)
	if err != nil {
		return nil, err
	}

	iconURL, err := validateOptionalHTTPURL(input.IconURL, "iconUrl")
	if err != nil {
		return nil, err
	}

	guildID, err := idgen.NewUUIDv7()
	if err != nil {
		return nil, fmt.Errorf("generate guild id: %w", err)
	}

	channelID, err := idgen.NewUUIDv7()
	if err != nil {
		return nil, fmt.Errorf("generate default channel id: %w", err)
	}

	guild := &domain.Guild{
		ID:      guildID,
		Name:    name,
		IconURL: iconURL,
		OwnerID: ownerID,
	}
	defaultChannel := &domain.Channel{
		ID:       channelID,
		GuildID:  guildID,
		Name:     "general",
		Type:     0,
		Position: 0,
	}
	ownerMember := &domain.GuildMember{
		GuildID: guildID,
		UserID:  ownerID,
		Role:    domain.RoleOwner,
	}

	if err := s.guilds.CreateWithDefaultChannel(ctx, guild, defaultChannel, ownerMember); err != nil {
		return nil, err
	}

	s.subscriptions.Subscribe(ctx, ownerID, guild.ID)
	return guild, nil
}

// ListMyGuilds returns all guilds that userID is a member of.
func (s *GuildService) ListMyGuilds(ctx context.Context, userID uuid.UUID) ([]domain.Guild, error) {
	return s.guilds.ListByUser(ctx, userID)
}

// GetGuild returns guild details when userID is a member of guildID.
func (s *GuildService) GetGuild(ctx context.Context, userID, guildID uuid.UUID) (*domain.Guild, error) {
	guild, _, err := loadGuildAndMember(ctx, s.guilds, userID, guildID)
	if err != nil {
		return nil, err
	}
	return guild, nil
}

// UpdateGuild updates guild name and/or iconUrl; only the guild owner is permitted.
func (s *GuildService) UpdateGuild(ctx context.Context, userID, guildID uuid.UUID, input UpdateGuildInput) (*domain.Guild, error) {
	_, member, err := loadGuildAndMember(ctx, s.guilds, userID, guildID)
	if err != nil {
		return nil, err
	}
	if member.Role != domain.RoleOwner {
		return nil, domain.NewError(domain.ErrForbidden, "FORBIDDEN", "only guild owner can update the guild")
	}

	params := domain.UpdateGuildParams{
		IconURLSet: input.IconURLSet,
	}
	if input.Name != nil {
		name, err := validateGuildName(*input.Name)
		if err != nil {
			return nil, err
		}
		params.Name = &name
	}
	if input.IconURLSet {
		iconURL, err := validateOptionalHTTPURL(input.IconURL, "iconUrl")
		if err != nil {
			return nil, err
		}
		params.IconURL = iconURL
	}

	return s.guilds.Update(ctx, guildID, params)
}

// DeleteGuild deletes a guild; only the guild owner is permitted.
func (s *GuildService) DeleteGuild(ctx context.Context, userID, guildID uuid.UUID) error {
	_, member, err := loadGuildAndMember(ctx, s.guilds, userID, guildID)
	if err != nil {
		return err
	}
	if member.Role != domain.RoleOwner {
		return domain.NewError(domain.ErrForbidden, "FORBIDDEN", "only guild owner can delete the guild")
	}

	return s.guilds.Delete(ctx, guildID)
}

// ListMembers returns all members of guildID enriched with best-effort online status.
func (s *GuildService) ListMembers(ctx context.Context, userID, guildID uuid.UUID) ([]domain.GuildMember, error) {
	if _, _, err := loadGuildAndMember(ctx, s.guilds, userID, guildID); err != nil {
		return nil, err
	}

	members, err := s.guilds.ListMembers(ctx, guildID)
	if err != nil {
		return nil, err
	}

	if s.presence != nil && len(members) > 0 {
		userIDs := make([]uuid.UUID, 0, len(members))
		for _, m := range members {
			userIDs = append(userIDs, m.UserID)
		}
		if onlineMap, err := s.presence.GetOnlineMap(ctx, userIDs); err == nil {
			for i := range members {
				members[i].Online = onlineMap[members[i].UserID]
			}
		}
	}

	return members, nil
}

// LeaveGuild removes userID from guildID and unsubscribes active WebSocket sessions (ADR-007).
// Guild owners are blocked with 403 FORBIDDEN (ADR-006).
func (s *GuildService) LeaveGuild(ctx context.Context, userID, guildID uuid.UUID) error {
	_, member, err := loadGuildAndMember(ctx, s.guilds, userID, guildID)
	if err != nil {
		return err
	}
	if member.Role == domain.RoleOwner {
		return domain.NewError(domain.ErrForbidden, "FORBIDDEN", "guild owner cannot leave the guild; delete the guild instead")
	}

	if err := s.guilds.RemoveMember(ctx, guildID, userID); err != nil {
		return err
	}

	s.subscriptions.Unsubscribe(ctx, userID, guildID)
	return nil
}

func loadGuildAndMember(
	ctx context.Context,
	guilds domain.GuildRepository,
	userID, guildID uuid.UUID,
) (*domain.Guild, *domain.GuildMember, error) {
	guild, err := guilds.GetByID(ctx, guildID)
	if err != nil {
		return nil, nil, err
	}

	member, err := guilds.GetMember(ctx, guildID, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, nil, domain.NewError(domain.ErrForbidden, "FORBIDDEN", "you are not a member of this guild")
		}
		return nil, nil, err
	}

	return guild, member, nil
}

func validateGuildName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	n := utf8.RuneCountInString(name)
	if n < 1 || n > 100 {
		return "", domain.NewError(domain.ErrValidation, "VALIDATION_ERROR", "guild name must be between 1 and 100 characters")
	}
	return name, nil
}

func validateOptionalHTTPURL(raw *string, field string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		return nil, nil
	}
	if len(trimmed) > 2048 || !isValidHTTPURL(trimmed) {
		return nil, domain.NewError(domain.ErrValidation, "VALIDATION_ERROR", fmt.Sprintf("%s must be a valid http or https URL", field))
	}
	return &trimmed, nil
}
