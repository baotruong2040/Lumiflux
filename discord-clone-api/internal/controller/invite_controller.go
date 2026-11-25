package controller

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/truong/discord-clone-api/internal/controller/dto"
	"github.com/truong/discord-clone-api/internal/domain"
	"github.com/truong/discord-clone-api/internal/service"
)

// InviteUseCase defines service operations required by InviteController.
type InviteUseCase interface {
	CreateInvite(ctx context.Context, userID, guildID uuid.UUID, input service.CreateInviteInput) (*domain.Invite, error)
	AcceptInvite(ctx context.Context, userID uuid.UUID, code string) (*domain.Guild, error)
}

// InviteController handles guild invite creation and acceptance endpoints.
type InviteController struct {
	invites       InviteUseCase
	extractUserID UserIDExtractor
}

// NewInviteController constructs an InviteController.
func NewInviteController(invites InviteUseCase, extractUserID UserIDExtractor) *InviteController {
	return &InviteController{
		invites:       invites,
		extractUserID: extractUserID,
	}
}

// Create handles POST /api/v1/guilds/:guildId/invites.
func (ctrl *InviteController) Create(c *gin.Context) {
	userID, ok := ctrl.extractUserID(c)
	if !ok {
		WriteErrorStatus(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	guildID, err := uuid.Parse(c.Param("guildId"))
	if err != nil {
		WriteErrorStatus(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid guildId parameter")
		return
	}

	var req dto.CreateInviteRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		WriteErrorStatus(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON request body")
		return
	}

	invite, err := ctrl.invites.CreateInvite(c.Request.Context(), userID, guildID, service.CreateInviteInput{
		MaxUses:           req.MaxUses,
		ExpiresInHours:    req.ExpiresInHours.Value,
		ExpiresInHoursSet: req.ExpiresInHours.Set,
	})
	if err != nil {
		WriteError(c, err)
		return
	}

	c.JSON(http.StatusCreated, dto.ToInviteResponse(*invite))
}

// Accept handles POST /api/v1/invites/:code/accept.
func (ctrl *InviteController) Accept(c *gin.Context) {
	userID, ok := ctrl.extractUserID(c)
	if !ok {
		WriteErrorStatus(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	guild, err := ctrl.invites.AcceptInvite(c.Request.Context(), userID, c.Param("code"))
	if err != nil {
		WriteError(c, err)
		return
	}

	c.JSON(http.StatusOK, dto.ToGuildResponse(*guild))
}
