package domain

import (
	"context"

	"github.com/google/uuid"
)

// ChannelEventPublisher emits realtime channel lifecycle events (ADR-003).
type ChannelEventPublisher interface {
	PublishChannelCreate(ctx context.Context, guildID uuid.UUID, channel Channel)
	PublishChannelUpdate(ctx context.Context, guildID uuid.UUID, channel Channel)
	PublishChannelDelete(ctx context.Context, guildID uuid.UUID, channel Channel)
}

// NoopChannelEventPublisher is used when realtime channel broadcasting is disabled.
type NoopChannelEventPublisher struct{}

// PublishChannelCreate is a no-op placeholder.
func (NoopChannelEventPublisher) PublishChannelCreate(_ context.Context, _ uuid.UUID, _ Channel) {}

// PublishChannelUpdate is a no-op placeholder.
func (NoopChannelEventPublisher) PublishChannelUpdate(_ context.Context, _ uuid.UUID, _ Channel) {}

// PublishChannelDelete is a no-op placeholder.
func (NoopChannelEventPublisher) PublishChannelDelete(_ context.Context, _ uuid.UUID, _ Channel) {}

// GuildEventPublisher emits realtime guild membership and deletion events (ADR-003, ADR-007).
type GuildEventPublisher interface {
	PublishGuildMemberAdd(ctx context.Context, guildID uuid.UUID, member GuildMember)
	PublishGuildMemberRemove(ctx context.Context, guildID uuid.UUID, user User)
	PublishGuildDelete(ctx context.Context, guildID uuid.UUID)
}

// NoopGuildEventPublisher is used when realtime guild broadcasting is disabled.
type NoopGuildEventPublisher struct{}

// PublishGuildMemberAdd is a no-op placeholder.
func (NoopGuildEventPublisher) PublishGuildMemberAdd(_ context.Context, _ uuid.UUID, _ GuildMember) {
}

// PublishGuildMemberRemove is a no-op placeholder.
func (NoopGuildEventPublisher) PublishGuildMemberRemove(_ context.Context, _ uuid.UUID, _ User) {}

// PublishGuildDelete is a no-op placeholder.
func (NoopGuildEventPublisher) PublishGuildDelete(_ context.Context, _ uuid.UUID) {}
