package dto

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/truong/discord-clone-api/internal/domain"
)

// NullableInt distinguishes omitted JSON fields from explicit null or integer values.
type NullableInt struct {
	Set   bool
	Value *int
}

// UnmarshalJSON tracks whether the integer field was explicitly present in JSON.
func (n *NullableInt) UnmarshalJSON(data []byte) error {
	n.Set = true
	trimmed := bytes.TrimSpace(data)
	if bytes.Equal(trimmed, []byte("null")) {
		n.Value = nil
		return nil
	}
	var v int
	if err := json.Unmarshal(trimmed, &v); err != nil {
		return err
	}
	n.Value = &v
	return nil
}

// CreateGuildRequest represents the JSON body for POST /api/v1/guilds.
type CreateGuildRequest struct {
	Name    string  `json:"name"`
	IconURL *string `json:"iconUrl"`
}

// UpdateGuildRequest represents the JSON body for PATCH /api/v1/guilds/:guildId.
type UpdateGuildRequest struct {
	Name    *string        `json:"name"`
	IconURL NullableString `json:"iconUrl"`
}

// CreateInviteRequest represents the optional JSON body for POST /api/v1/guilds/:guildId/invites.
type CreateInviteRequest struct {
	MaxUses        *int        `json:"maxUses"`
	ExpiresInHours NullableInt `json:"expiresInHours"`
}

// GuildResponse matches the Guild schema in api/openapi.yaml.
type GuildResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	IconURL   *string   `json:"iconUrl"`
	OwnerID   uuid.UUID `json:"ownerId"`
	CreatedAt time.Time `json:"createdAt"`
}

// GuildMemberResponse matches the GuildMember schema in api/openapi.yaml.
type GuildMemberResponse struct {
	GuildID  uuid.UUID    `json:"guildId"`
	UserID   uuid.UUID    `json:"userId"`
	Nickname *string      `json:"nickname"`
	Role     string       `json:"role"`
	Online   bool         `json:"online"`
	JoinedAt time.Time    `json:"joinedAt"`
	User     UserResponse `json:"user"`
}

// InviteResponse matches the Invite schema in api/openapi.yaml.
type InviteResponse struct {
	Code      string     `json:"code"`
	GuildID   uuid.UUID  `json:"guildId"`
	CreatedBy uuid.UUID  `json:"createdBy"`
	MaxUses   *int       `json:"maxUses"`
	Uses      int        `json:"uses"`
	ExpiresAt *time.Time `json:"expiresAt"`
	CreatedAt time.Time  `json:"createdAt"`
}

// ToGuildResponse converts a domain Guild into a GuildResponse DTO.
func ToGuildResponse(g domain.Guild) GuildResponse {
	return GuildResponse{
		ID:        g.ID,
		Name:      g.Name,
		IconURL:   g.IconURL,
		OwnerID:   g.OwnerID,
		CreatedAt: g.CreatedAt,
	}
}

// ToGuildResponses converts a slice of domain Guilds into GuildResponse DTOs.
func ToGuildResponses(guilds []domain.Guild) []GuildResponse {
	out := make([]GuildResponse, 0, len(guilds))
	for _, g := range guilds {
		out = append(out, ToGuildResponse(g))
	}
	return out
}

// ToGuildMemberResponse converts a domain GuildMember into a GuildMemberResponse DTO.
func ToGuildMemberResponse(m domain.GuildMember) GuildMemberResponse {
	return GuildMemberResponse{
		GuildID:  m.GuildID,
		UserID:   m.UserID,
		Nickname: m.Nickname,
		Role:     string(m.Role),
		Online:   m.Online,
		JoinedAt: m.JoinedAt,
		User:     ToUserResponse(m.User),
	}
}

// ToGuildMemberResponses converts a slice of domain GuildMembers into GuildMemberResponse DTOs.
func ToGuildMemberResponses(members []domain.GuildMember) []GuildMemberResponse {
	out := make([]GuildMemberResponse, 0, len(members))
	for _, m := range members {
		out = append(out, ToGuildMemberResponse(m))
	}
	return out
}

// ToInviteResponse converts a domain Invite into an InviteResponse DTO.
func ToInviteResponse(inv domain.Invite) InviteResponse {
	return InviteResponse{
		Code:      inv.Code,
		GuildID:   inv.GuildID,
		CreatedBy: inv.CreatedBy,
		MaxUses:   inv.MaxUses,
		Uses:      inv.Uses,
		ExpiresAt: inv.ExpiresAt,
		CreatedAt: inv.CreatedAt,
	}
}
