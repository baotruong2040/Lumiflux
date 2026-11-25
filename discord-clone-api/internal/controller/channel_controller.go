package controller

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/truong/discord-clone-api/internal/controller/dto"
	"github.com/truong/discord-clone-api/internal/domain"
	"github.com/truong/discord-clone-api/internal/service"
)

// ChannelUseCase defines service operations required by ChannelController.
type ChannelUseCase interface {
	ListChannels(ctx context.Context, userID, guildID uuid.UUID) ([]domain.Channel, error)
	CreateChannel(ctx context.Context, userID, guildID uuid.UUID, input service.CreateChannelInput) (*domain.Channel, error)
	UpdateChannel(ctx context.Context, userID, channelID uuid.UUID, input service.UpdateChannelInput) (*domain.Channel, error)
	DeleteChannel(ctx context.Context, userID, channelID uuid.UUID) error
}

// ChannelController handles guild channel HTTP endpoints.
type ChannelController struct {
	channels      ChannelUseCase
	extractUserID UserIDExtractor
}

// NewChannelController constructs a ChannelController.
func NewChannelController(channels ChannelUseCase, extractUserID UserIDExtractor) *ChannelController {
	return &ChannelController{
		channels:      channels,
		extractUserID: extractUserID,
	}
}

// List handles GET /api/v1/guilds/:guildId/channels.
func (ctrl *ChannelController) List(c *gin.Context) {
	userID, guildID, ok := ctrl.requireUserAndUUIDParam(c, "guildId")
	if !ok {
		return
	}

	channels, err := ctrl.channels.ListChannels(c.Request.Context(), userID, guildID)
	if err != nil {
		WriteError(c, err)
		return
	}

	c.JSON(http.StatusOK, dto.ToChannelResponses(channels))
}

// Create handles POST /api/v1/guilds/:guildId/channels.
func (ctrl *ChannelController) Create(c *gin.Context) {
	userID, guildID, ok := ctrl.requireUserAndUUIDParam(c, "guildId")
	if !ok {
		return
	}

	var req dto.CreateChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		WriteErrorStatus(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON request body")
		return
	}

	ch, err := ctrl.channels.CreateChannel(c.Request.Context(), userID, guildID, service.CreateChannelInput{
		Name:  req.Name,
		Topic: req.Topic,
	})
	if err != nil {
		WriteError(c, err)
		return
	}

	c.JSON(http.StatusCreated, dto.ToChannelResponse(*ch))
}

// Update handles PATCH /api/v1/channels/:channelId.
func (ctrl *ChannelController) Update(c *gin.Context) {
	userID, channelID, ok := ctrl.requireUserAndUUIDParam(c, "channelId")
	if !ok {
		return
	}

	var req dto.UpdateChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		WriteErrorStatus(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON request body")
		return
	}

	ch, err := ctrl.channels.UpdateChannel(c.Request.Context(), userID, channelID, service.UpdateChannelInput{
		Name:     req.Name,
		Topic:    req.Topic.Value,
		TopicSet: req.Topic.Set,
	})
	if err != nil {
		WriteError(c, err)
		return
	}

	c.JSON(http.StatusOK, dto.ToChannelResponse(*ch))
}

// Delete handles DELETE /api/v1/channels/:channelId.
func (ctrl *ChannelController) Delete(c *gin.Context) {
	userID, channelID, ok := ctrl.requireUserAndUUIDParam(c, "channelId")
	if !ok {
		return
	}

	if err := ctrl.channels.DeleteChannel(c.Request.Context(), userID, channelID); err != nil {
		WriteError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

func (ctrl *ChannelController) requireUserAndUUIDParam(c *gin.Context, param string) (uuid.UUID, uuid.UUID, bool) {
	userID, ok := ctrl.extractUserID(c)
	if !ok {
		WriteErrorStatus(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return uuid.Nil, uuid.Nil, false
	}

	parsedID, err := uuid.Parse(c.Param(param))
	if err != nil {
		WriteErrorStatus(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid "+param+" parameter")
		return uuid.Nil, uuid.Nil, false
	}

	return userID, parsedID, true
}
