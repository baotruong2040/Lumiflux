package service

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/truong/discord-clone-api/internal/domain"
	"github.com/truong/discord-clone-api/pkg/idgen"
)

// CreateChannelInput carries fields for creating a channel inside a guild.
type CreateChannelInput struct {
	Name  string
	Topic *string
}

// UpdateChannelInput carries mutable fields for updating a channel.
type UpdateChannelInput struct {
	Name     *string
	Topic    *string
	TopicSet bool
}

// ChannelService implements guild channel CRUD with membership and owner/admin RBAC checks.
type ChannelService struct {
	guilds   domain.GuildRepository
	channels domain.ChannelRepository
}

// NewChannelService constructs a ChannelService.
func NewChannelService(guilds domain.GuildRepository, channels domain.ChannelRepository) *ChannelService {
	return &ChannelService{
		guilds:   guilds,
		channels: channels,
	}
}

// ListChannels returns all channels in guildID for members of that guild.
func (s *ChannelService) ListChannels(ctx context.Context, userID, guildID uuid.UUID) ([]domain.Channel, error) {
	if _, _, err := loadGuildAndMember(ctx, s.guilds, userID, guildID); err != nil {
		return nil, err
	}
	return s.channels.ListByGuild(ctx, guildID)
}

// CreateChannel creates a text channel inside guildID; only owner or admin is permitted.
func (s *ChannelService) CreateChannel(ctx context.Context, userID, guildID uuid.UUID, input CreateChannelInput) (*domain.Channel, error) {
	_, member, err := loadGuildAndMember(ctx, s.guilds, userID, guildID)
	if err != nil {
		return nil, err
	}
	if !member.Role.CanManageChannels() {
		return nil, domain.NewError(domain.ErrForbidden, "FORBIDDEN", "only guild owner or admin can create channels")
	}

	name, err := validateChannelName(input.Name)
	if err != nil {
		return nil, err
	}
	topic, err := validateChannelTopic(input.Topic)
	if err != nil {
		return nil, err
	}

	channelID, err := idgen.NewUUIDv7()
	if err != nil {
		return nil, fmt.Errorf("generate channel id: %w", err)
	}

	ch := &domain.Channel{
		ID:      channelID,
		GuildID: guildID,
		Name:    name,
		Type:    0,
		Topic:   topic,
	}
	if err := s.channels.Create(ctx, ch); err != nil {
		return nil, err
	}
	return ch, nil
}

// UpdateChannel updates a channel's name and/or topic; only owner or admin is permitted.
func (s *ChannelService) UpdateChannel(ctx context.Context, userID, channelID uuid.UUID, input UpdateChannelInput) (*domain.Channel, error) {
	ch, err := s.channels.GetByID(ctx, channelID)
	if err != nil {
		return nil, err
	}

	_, member, err := loadGuildAndMember(ctx, s.guilds, userID, ch.GuildID)
	if err != nil {
		return nil, err
	}
	if !member.Role.CanManageChannels() {
		return nil, domain.NewError(domain.ErrForbidden, "FORBIDDEN", "only guild owner or admin can update channels")
	}

	params := domain.UpdateChannelParams{
		TopicSet: input.TopicSet,
	}
	if input.Name != nil {
		name, err := validateChannelName(*input.Name)
		if err != nil {
			return nil, err
		}
		params.Name = &name
	}
	if input.TopicSet {
		topic, err := validateChannelTopic(input.Topic)
		if err != nil {
			return nil, err
		}
		params.Topic = topic
	}

	return s.channels.Update(ctx, channelID, params)
}

// DeleteChannel deletes a channel; only owner or admin is permitted.
func (s *ChannelService) DeleteChannel(ctx context.Context, userID, channelID uuid.UUID) error {
	ch, err := s.channels.GetByID(ctx, channelID)
	if err != nil {
		return err
	}

	_, member, err := loadGuildAndMember(ctx, s.guilds, userID, ch.GuildID)
	if err != nil {
		return err
	}
	if !member.Role.CanManageChannels() {
		return domain.NewError(domain.ErrForbidden, "FORBIDDEN", "only guild owner or admin can delete channels")
	}

	return s.channels.Delete(ctx, channelID)
}

func validateChannelName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	n := utf8.RuneCountInString(name)
	if n < 1 || n > 100 {
		return "", domain.NewError(domain.ErrValidation, "VALIDATION_ERROR", "channel name must be between 1 and 100 characters")
	}
	return name, nil
}

func validateChannelTopic(raw *string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(trimmed) > 255 {
		return nil, domain.NewError(domain.ErrValidation, "VALIDATION_ERROR", "channel topic must not exceed 255 characters")
	}
	return &trimmed, nil
}
