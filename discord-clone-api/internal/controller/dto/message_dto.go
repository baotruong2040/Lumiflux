package dto

import (
	"time"

	"github.com/google/uuid"
	"github.com/truong/discord-clone-api/internal/domain"
)

// SendMessageRequest represents the JSON body for POST /api/v1/channels/:channelId/messages.
type SendMessageRequest struct {
	Content string  `json:"content"`
	Nonce   *string `json:"nonce"`
}

// UpdateMessageRequest represents the JSON body for PATCH /api/v1/channels/:channelId/messages/:messageId.
type UpdateMessageRequest struct {
	Content string `json:"content"`
}

// MessageAuthorResponse matches the MessageAuthor schema in api/openapi.yaml.
type MessageAuthorResponse struct {
	ID          uuid.UUID `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"displayName"`
	AvatarURL   *string   `json:"avatarUrl"`
}

// MessageResponse matches the Message schema in api/openapi.yaml.
type MessageResponse struct {
	ID        uuid.UUID             `json:"id"`
	ChannelID uuid.UUID             `json:"channelId"`
	Author    MessageAuthorResponse `json:"author"`
	Content   string                `json:"content"`
	Nonce     *string               `json:"nonce"`
	EditedAt  *time.Time            `json:"editedAt"`
	CreatedAt time.Time             `json:"createdAt"`
}

// ToMessageResponse converts a domain Message into a MessageResponse DTO.
func ToMessageResponse(m domain.Message) MessageResponse {
	return MessageResponse{
		ID:        m.ID,
		ChannelID: m.ChannelID,
		Author: MessageAuthorResponse{
			ID:          m.Author.ID,
			Username:    m.Author.Username,
			DisplayName: m.Author.DisplayName,
			AvatarURL:   m.Author.AvatarURL,
		},
		Content:   m.Content,
		Nonce:     m.Nonce,
		EditedAt:  m.EditedAt,
		CreatedAt: m.CreatedAt,
	}
}

// ToMessageResponses converts a slice of domain Messages into MessageResponse DTOs.
func ToMessageResponses(messages []domain.Message) []MessageResponse {
	out := make([]MessageResponse, 0, len(messages))
	for _, m := range messages {
		out = append(out, ToMessageResponse(m))
	}
	return out
}
