package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Role represents a user's permission role within a guild.
type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

// IsValid reports whether r is a recognized guild member role.
func (r Role) IsValid() bool {
	return r == RoleOwner || r == RoleAdmin || r == RoleMember
}

// Guild represents a Discord-style server.
type Guild struct {
	ID        uuid.UUID
	Name      string
	IconURL   *string
	OwnerID   uuid.UUID
	CreatedAt time.Time
}

// UpdateGuildParams specifies mutable fields for a partial guild update.
type UpdateGuildParams struct {
	Name       *string
	IconURL    *string
	IconURLSet bool
}

// Channel represents a communication channel inside a guild.
type Channel struct {
	ID        uuid.UUID
	GuildID   uuid.UUID
	Name      string
	Type      int16
	Topic     *string
	Position  int
	CreatedAt time.Time
}

// GuildMember links a user to a guild with role, nickname, and presence state.
type GuildMember struct {
	GuildID  uuid.UUID
	UserID   uuid.UUID
	Nickname *string
	Role     Role
	Online   bool
	JoinedAt time.Time
	User     User
}

// Invite represents a shareable code allowing users to join a guild.
type Invite struct {
	Code      string
	GuildID   uuid.UUID
	CreatedBy uuid.UUID
	MaxUses   *int
	Uses      int
	ExpiresAt *time.Time
	CreatedAt time.Time
}

// NoopGuildSubscriptionManager is a no-op GuildSubscriptionManager used before the WebSocket Hub is wired.
type NoopGuildSubscriptionManager struct{}

// Subscribe is a no-op placeholder.
func (NoopGuildSubscriptionManager) Subscribe(_ context.Context, _, _ uuid.UUID) {}

// Unsubscribe is a no-op placeholder for ADR-007.
func (NoopGuildSubscriptionManager) Unsubscribe(_ context.Context, _, _ uuid.UUID) {}
