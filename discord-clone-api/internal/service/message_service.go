package service

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/truong/discord-clone-api/internal/domain"
	"github.com/truong/discord-clone-api/pkg/idgen"
	"github.com/truong/discord-clone-api/pkg/logger"
)

// SendMessageInput carries validated inputs for creating a new message.
type SendMessageInput struct {
	Content string
	Nonce   *string
}

// ListMessagesInput specifies cursor pagination parameters for listing channel messages.
type ListMessagesInput struct {
	Before *uuid.UUID
	Limit  int
}

// UpdateMessageInput carries the updated message text.
type UpdateMessageInput struct {
	Content string
}

// MessageService implements channel message history, creation with rate limiting, editing, and soft deletion.
type MessageService struct {
	guilds      domain.GuildRepository
	channels    domain.ChannelRepository
	messages    domain.MessageRepository
	rateLimiter domain.RateLimiter
	publisher   domain.MessageEventPublisher
	rateLimit   int
	rateWindow  time.Duration
}

// NewMessageService constructs a MessageService.
func NewMessageService(
	guilds domain.GuildRepository,
	channels domain.ChannelRepository,
	messages domain.MessageRepository,
	rateLimiter domain.RateLimiter,
	publisher domain.MessageEventPublisher,
	rateLimit int,
	rateWindow time.Duration,
) *MessageService {
	if publisher == nil {
		publisher = domain.NoopMessageEventPublisher{}
	}
	return &MessageService{
		guilds:      guilds,
		channels:    channels,
		messages:    messages,
		rateLimiter: rateLimiter,
		publisher:   publisher,
		rateLimit:   rateLimit,
		rateWindow:  rateWindow,
	}
}

// ListMessages returns cursor-paginated non-deleted messages in channelID for guild members.
func (s *MessageService) ListMessages(
	ctx context.Context,
	userID, channelID uuid.UUID,
	input ListMessagesInput,
) ([]domain.Message, error) {
	ch, err := s.channels.GetByID(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if _, _, err := loadGuildAndMember(ctx, s.guilds, userID, ch.GuildID); err != nil {
		return nil, err
	}

	limit := input.Limit
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 100 {
		return nil, domain.NewError(domain.ErrValidation, "VALIDATION_ERROR", "limit must be between 1 and 100")
	}

	return s.messages.ListByChannel(ctx, channelID, input.Before, limit)
}

// SendMessage creates a message after checking membership, content/nonce validation, and user rate limit.
func (s *MessageService) SendMessage(
	ctx context.Context,
	userID, channelID uuid.UUID,
	input SendMessageInput,
) (*domain.Message, error) {
	ch, err := s.channels.GetByID(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if _, _, err := loadGuildAndMember(ctx, s.guilds, userID, ch.GuildID); err != nil {
		return nil, err
	}

	content, err := validateMessageContent(input.Content)
	if err != nil {
		return nil, err
	}
	nonce, err := validateNonce(input.Nonce)
	if err != nil {
		return nil, err
	}

	if s.rateLimiter != nil && s.rateLimit > 0 {
		key := "rl:" + userID.String() + ":msg"
		allowed, retryAfter, rlErr := s.rateLimiter.Allow(ctx, key, s.rateLimit, s.rateWindow)
		if rlErr != nil {
			logger.FromContext(ctx).Warn("message rate limiter error, allowing request", "key", key, "err", rlErr)
		} else if !allowed {
			return nil, domain.NewRateLimitError(retryAfter, "message rate limit exceeded, please try again later")
		}
	}

	msgID, err := idgen.NewUUIDv7()
	if err != nil {
		return nil, fmt.Errorf("generate message id: %w", err)
	}

	created, err := s.messages.Create(ctx, &domain.Message{
		ID:        msgID,
		ChannelID: channelID,
		AuthorID:  userID,
		Content:   content,
		Nonce:     nonce,
	})
	if err != nil {
		return nil, err
	}

	s.publisher.PublishMessageCreate(ctx, ch.GuildID, *created)
	return created, nil
}

// UpdateMessage edits the content of an existing message; only the message author is permitted.
func (s *MessageService) UpdateMessage(
	ctx context.Context,
	userID, channelID, messageID uuid.UUID,
	input UpdateMessageInput,
) (*domain.Message, error) {
	ch, err := s.channels.GetByID(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if _, _, err := loadGuildAndMember(ctx, s.guilds, userID, ch.GuildID); err != nil {
		return nil, err
	}

	existing, err := s.messages.GetByID(ctx, messageID)
	if err != nil {
		return nil, err
	}
	if existing.ChannelID != channelID {
		return nil, domain.NewError(domain.ErrNotFound, "MESSAGE_NOT_FOUND", "message not found")
	}
	if existing.AuthorID != userID {
		return nil, domain.NewError(domain.ErrForbidden, "FORBIDDEN", "only the message author can edit this message")
	}

	content, err := validateMessageContent(input.Content)
	if err != nil {
		return nil, err
	}

	updated, err := s.messages.UpdateContent(ctx, messageID, content)
	if err != nil {
		return nil, err
	}

	s.publisher.PublishMessageUpdate(ctx, ch.GuildID, *updated)
	return updated, nil
}

// DeleteMessage soft-deletes a message; permitted for the message author or guild owner/admin.
func (s *MessageService) DeleteMessage(
	ctx context.Context,
	userID, channelID, messageID uuid.UUID,
) error {
	ch, err := s.channels.GetByID(ctx, channelID)
	if err != nil {
		return err
	}
	_, member, err := loadGuildAndMember(ctx, s.guilds, userID, ch.GuildID)
	if err != nil {
		return err
	}

	existing, err := s.messages.GetByID(ctx, messageID)
	if err != nil {
		return err
	}
	if existing.ChannelID != channelID {
		return domain.NewError(domain.ErrNotFound, "MESSAGE_NOT_FOUND", "message not found")
	}
	if existing.AuthorID != userID && !member.Role.CanManageChannels() {
		return domain.NewError(domain.ErrForbidden, "FORBIDDEN", "you do not have permission to delete this message")
	}

	if err := s.messages.SoftDelete(ctx, messageID); err != nil {
		return err
	}

	s.publisher.PublishMessageDelete(ctx, ch.GuildID, channelID, messageID)
	return nil
}

func validateMessageContent(raw string) (string, error) {
	content := strings.TrimSpace(raw)
	n := utf8.RuneCountInString(content)
	if n < 1 || n > 2000 {
		return "", domain.NewError(domain.ErrValidation, "VALIDATION_ERROR", "message content must be between 1 and 2000 characters")
	}
	return content, nil
}

func validateNonce(raw *string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(trimmed) > 64 {
		return nil, domain.NewError(domain.ErrValidation, "VALIDATION_ERROR", "nonce must not exceed 64 characters")
	}
	return &trimmed, nil
}
