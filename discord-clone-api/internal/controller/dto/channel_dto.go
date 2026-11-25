package dto

import (
	"time"

	"github.com/google/uuid"
	"github.com/truong/discord-clone-api/internal/domain"
)

// CreateChannelRequest represents the JSON body for POST /api/v1/guilds/:guildId/channels.
type CreateChannelRequest struct {
	Name  string  `json:"name"`
	Topic *string `json:"topic"`
}

// UpdateChannelRequest represents the JSON body for PATCH /api/v1/channels/:channelId.
type UpdateChannelRequest struct {
	Name  *string        `json:"name"`
	Topic NullableString `json:"topic"`
}

// ChannelResponse matches the Channel schema in api/openapi.yaml.
type ChannelResponse struct {
	ID        uuid.UUID `json:"id"`
	GuildID   uuid.UUID `json:"guildId"`
	Name      string    `json:"name"`
	Type      int16     `json:"type"`
	Topic     *string   `json:"topic"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"createdAt"`
}

// ToChannelResponse converts a domain Channel into a ChannelResponse DTO.
func ToChannelResponse(ch domain.Channel) ChannelResponse {
	return ChannelResponse{
		ID:        ch.ID,
		GuildID:   ch.GuildID,
		Name:      ch.Name,
		Type:      ch.Type,
		Topic:     ch.Topic,
		Position:  ch.Position,
		CreatedAt: ch.CreatedAt,
	}
}

// ToChannelResponses converts a slice of domain Channels into ChannelResponse DTOs.
func ToChannelResponses(channels []domain.Channel) []ChannelResponse {
	out := make([]ChannelResponse, 0, len(channels))
	for _, ch := range channels {
		out = append(out, ToChannelResponse(ch))
	}
	return out
}
