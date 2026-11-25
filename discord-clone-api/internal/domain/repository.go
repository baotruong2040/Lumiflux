package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// UserRepository defines persistence operations for User entities.
type UserRepository interface {
	Create(ctx context.Context, user *User) error
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetByLogin(ctx context.Context, login string) (*User, error)
	Update(ctx context.Context, id uuid.UUID, params UpdateUserParams) (*User, error)
}

// TokenStore defines ephemeral storage operations for refresh token rotation and revocation.
type TokenStore interface {
	SaveRefreshToken(ctx context.Context, tokenHash string, userID uuid.UUID, ttl time.Duration) error
	ConsumeRefreshToken(ctx context.Context, tokenHash string) (uuid.UUID, error)
	DeleteRefreshToken(ctx context.Context, tokenHash string) error
}

// RateLimiter checks and increments sliding/fixed-window request counters.
type RateLimiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (allowed bool, retryAfter time.Duration, err error)
}

// PasswordHasher abstracts password hashing and verification.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(hash, password string) error
}

// TokenManager abstracts JWT access token and opaque refresh token operations.
type TokenManager interface {
	GenerateAccessToken(userID uuid.UUID) (string, error)
	ParseAccessToken(tokenStr string) (uuid.UUID, error)
	GenerateRefreshToken() (raw string, hash string, err error)
	HashRefreshToken(raw string) string
}

// HealthChecker verifies connectivity to an external dependency.
type HealthChecker interface {
	Ping(ctx context.Context) error
}

// GuildRepository defines persistence operations for guilds, default channel creation, and memberships.
type GuildRepository interface {
	CreateWithDefaultChannel(ctx context.Context, guild *Guild, defaultChannel *Channel, ownerMember *GuildMember) error
	ListByUser(ctx context.Context, userID uuid.UUID) ([]Guild, error)
	GetByID(ctx context.Context, guildID uuid.UUID) (*Guild, error)
	Update(ctx context.Context, guildID uuid.UUID, params UpdateGuildParams) (*Guild, error)
	Delete(ctx context.Context, guildID uuid.UUID) error
	GetMember(ctx context.Context, guildID, userID uuid.UUID) (*GuildMember, error)
	ListMembers(ctx context.Context, guildID uuid.UUID) ([]GuildMember, error)
	RemoveMember(ctx context.Context, guildID, userID uuid.UUID) error
}

// ChannelRepository defines persistence operations for channels within guilds.
type ChannelRepository interface {
	Create(ctx context.Context, channel *Channel) error
	ListByGuild(ctx context.Context, guildID uuid.UUID) ([]Channel, error)
	GetByID(ctx context.Context, channelID uuid.UUID) (*Channel, error)
	Update(ctx context.Context, channelID uuid.UUID, params UpdateChannelParams) (*Channel, error)
	Delete(ctx context.Context, channelID uuid.UUID) error
}

// InviteRepository defines persistence operations for guild invites.
type InviteRepository interface {
	Create(ctx context.Context, invite *Invite) error
	Accept(ctx context.Context, code string, userID uuid.UUID, now time.Time) (*Guild, *GuildMember, error)
}

// GuildSubscriptionManager hooks membership changes into realtime WebSocket Hub subscriptions (ADR-007).
type GuildSubscriptionManager interface {
	Subscribe(ctx context.Context, userID, guildID uuid.UUID)
	Unsubscribe(ctx context.Context, userID, guildID uuid.UUID)
}

// PresenceReader fetches online status for a batch of user IDs.
type PresenceReader interface {
	GetOnlineMap(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]bool, error)
}

// MessageRepository defines persistence operations for channel messages.
type MessageRepository interface {
	Create(ctx context.Context, msg *Message) (*Message, error)
	ListByChannel(ctx context.Context, channelID uuid.UUID, before *uuid.UUID, limit int) ([]Message, error)
	GetByID(ctx context.Context, messageID uuid.UUID) (*Message, error)
	UpdateContent(ctx context.Context, messageID uuid.UUID, content string) (*Message, error)
	SoftDelete(ctx context.Context, messageID uuid.UUID) error
}
