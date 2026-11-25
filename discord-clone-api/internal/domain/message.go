package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// MessageAuthor carries the subset of user profile fields embedded in message payloads.
type MessageAuthor struct {
	ID          uuid.UUID
	Username    string
	DisplayName string
	AvatarURL   *string
}

// Message represents a text message posted inside a guild channel.
type Message struct {
	ID        uuid.UUID
	ChannelID uuid.UUID
	AuthorID  uuid.UUID
	Author    MessageAuthor
	Content   string
	Nonce     *string
	EditedAt  *time.Time
	DeletedAt *time.Time
	CreatedAt time.Time
}

// MessageEventPublisher emits realtime message lifecycle events (ADR-003, wired in Day 12-13).
type MessageEventPublisher interface {
	PublishMessageCreate(ctx context.Context, guildID uuid.UUID, msg Message)
	PublishMessageUpdate(ctx context.Context, guildID uuid.UUID, msg Message)
	PublishMessageDelete(ctx context.Context, guildID, channelID, messageID uuid.UUID)
}

// NoopMessageEventPublisher is used before the Redis Pub/Sub Gateway publisher is wired.
type NoopMessageEventPublisher struct{}

// PublishMessageCreate is a no-op placeholder.
func (NoopMessageEventPublisher) PublishMessageCreate(_ context.Context, _ uuid.UUID, _ Message) {}

// PublishMessageUpdate is a no-op placeholder.
func (NoopMessageEventPublisher) PublishMessageUpdate(_ context.Context, _ uuid.UUID, _ Message) {}

// PublishMessageDelete is a no-op placeholder.
func (NoopMessageEventPublisher) PublishMessageDelete(_ context.Context, _, _, _ uuid.UUID) {}
