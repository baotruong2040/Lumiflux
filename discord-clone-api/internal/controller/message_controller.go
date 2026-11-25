package controller

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/truong/discord-clone-api/internal/controller/dto"
	"github.com/truong/discord-clone-api/internal/domain"
	"github.com/truong/discord-clone-api/internal/service"
)

// MessageUseCase defines service operations required by MessageController.
type MessageUseCase interface {
	ListMessages(ctx context.Context, userID, channelID uuid.UUID, input service.ListMessagesInput) ([]domain.Message, error)
	SendMessage(ctx context.Context, userID, channelID uuid.UUID, input service.SendMessageInput) (*domain.Message, error)
	UpdateMessage(ctx context.Context, userID, channelID, messageID uuid.UUID, input service.UpdateMessageInput) (*domain.Message, error)
	DeleteMessage(ctx context.Context, userID, channelID, messageID uuid.UUID) error
}

// MessageController handles channel message HTTP endpoints.
type MessageController struct {
	messages      MessageUseCase
	extractUserID UserIDExtractor
}

// NewMessageController constructs a MessageController.
func NewMessageController(messages MessageUseCase, extractUserID UserIDExtractor) *MessageController {
	return &MessageController{
		messages:      messages,
		extractUserID: extractUserID,
	}
}

// List handles GET /api/v1/channels/:channelId/messages.
func (ctrl *MessageController) List(c *gin.Context) {
	userID, channelID, ok := ctrl.requireUserAndChannelID(c)
	if !ok {
		return
	}

	var beforeID *uuid.UUID
	if beforeRaw := strings.TrimSpace(c.Query("before")); beforeRaw != "" {
		parsed, err := uuid.Parse(beforeRaw)
		if err != nil {
			WriteErrorStatus(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid before cursor parameter")
			return
		}
		beforeID = &parsed
	}

	limit := 50
	if limitRaw := strings.TrimSpace(c.Query("limit")); limitRaw != "" {
		parsedLimit, err := strconv.Atoi(limitRaw)
		if err != nil || parsedLimit < 1 || parsedLimit > 100 {
			WriteErrorStatus(c, http.StatusBadRequest, "VALIDATION_ERROR", "limit must be an integer between 1 and 100")
			return
		}
		limit = parsedLimit
	}

	messages, err := ctrl.messages.ListMessages(c.Request.Context(), userID, channelID, service.ListMessagesInput{
		Before: beforeID,
		Limit:  limit,
	})
	if err != nil {
		WriteError(c, err)
		return
	}

	c.JSON(http.StatusOK, dto.ToMessageResponses(messages))
}

// Send handles POST /api/v1/channels/:channelId/messages.
func (ctrl *MessageController) Send(c *gin.Context) {
	userID, channelID, ok := ctrl.requireUserAndChannelID(c)
	if !ok {
		return
	}

	var req dto.SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		WriteErrorStatus(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON request body")
		return
	}

	msg, err := ctrl.messages.SendMessage(c.Request.Context(), userID, channelID, service.SendMessageInput{
		Content: req.Content,
		Nonce:   req.Nonce,
	})
	if err != nil {
		WriteError(c, err)
		return
	}

	c.JSON(http.StatusCreated, dto.ToMessageResponse(*msg))
}

// Update handles PATCH /api/v1/channels/:channelId/messages/:messageId.
func (ctrl *MessageController) Update(c *gin.Context) {
	userID, channelID, messageID, ok := ctrl.requireUserChannelAndMessageID(c)
	if !ok {
		return
	}

	var req dto.UpdateMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		WriteErrorStatus(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON request body")
		return
	}

	msg, err := ctrl.messages.UpdateMessage(c.Request.Context(), userID, channelID, messageID, service.UpdateMessageInput{
		Content: req.Content,
	})
	if err != nil {
		WriteError(c, err)
		return
	}

	c.JSON(http.StatusOK, dto.ToMessageResponse(*msg))
}

// Delete handles DELETE /api/v1/channels/:channelId/messages/:messageId.
func (ctrl *MessageController) Delete(c *gin.Context) {
	userID, channelID, messageID, ok := ctrl.requireUserChannelAndMessageID(c)
	if !ok {
		return
	}

	if err := ctrl.messages.DeleteMessage(c.Request.Context(), userID, channelID, messageID); err != nil {
		WriteError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

func (ctrl *MessageController) requireUserAndChannelID(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	userID, ok := ctrl.extractUserID(c)
	if !ok {
		WriteErrorStatus(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return uuid.Nil, uuid.Nil, false
	}

	channelID, err := uuid.Parse(c.Param("channelId"))
	if err != nil {
		WriteErrorStatus(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid channelId parameter")
		return uuid.Nil, uuid.Nil, false
	}

	return userID, channelID, true
}

func (ctrl *MessageController) requireUserChannelAndMessageID(c *gin.Context) (uuid.UUID, uuid.UUID, uuid.UUID, bool) {
	userID, channelID, ok := ctrl.requireUserAndChannelID(c)
	if !ok {
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}

	messageID, err := uuid.Parse(c.Param("messageId"))
	if err != nil {
		WriteErrorStatus(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid messageId parameter")
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}

	return userID, channelID, messageID, true
}
