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

// GuildUseCase defines service operations required by GuildController.
type GuildUseCase interface {
	CreateGuild(ctx context.Context, ownerID uuid.UUID, input service.CreateGuildInput) (*domain.Guild, error)
	ListMyGuilds(ctx context.Context, userID uuid.UUID) ([]domain.Guild, error)
	GetGuild(ctx context.Context, userID, guildID uuid.UUID) (*domain.Guild, error)
	UpdateGuild(ctx context.Context, userID, guildID uuid.UUID, input service.UpdateGuildInput) (*domain.Guild, error)
	DeleteGuild(ctx context.Context, userID, guildID uuid.UUID) error
	ListMembers(ctx context.Context, userID, guildID uuid.UUID) ([]domain.GuildMember, error)
	LeaveGuild(ctx context.Context, userID, guildID uuid.UUID) error
}

// GuildController handles guild and membership HTTP endpoints.
type GuildController struct {
	guilds        GuildUseCase
	extractUserID UserIDExtractor
}

// NewGuildController constructs a GuildController.
func NewGuildController(guilds GuildUseCase, extractUserID UserIDExtractor) *GuildController {
	return &GuildController{
		guilds:        guilds,
		extractUserID: extractUserID,
	}
}

// Create handles POST /api/v1/guilds.
func (ctrl *GuildController) Create(c *gin.Context) {
	userID, ok := ctrl.extractUserID(c)
	if !ok {
		WriteErrorStatus(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req dto.CreateGuildRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		WriteErrorStatus(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON request body")
		return
	}

	guild, err := ctrl.guilds.CreateGuild(c.Request.Context(), userID, service.CreateGuildInput{
		Name:    req.Name,
		IconURL: req.IconURL,
	})
	if err != nil {
		WriteError(c, err)
		return
	}

	c.JSON(http.StatusCreated, dto.ToGuildResponse(*guild))
}

// List handles GET /api/v1/guilds.
func (ctrl *GuildController) List(c *gin.Context) {
	userID, ok := ctrl.extractUserID(c)
	if !ok {
		WriteErrorStatus(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	guilds, err := ctrl.guilds.ListMyGuilds(c.Request.Context(), userID)
	if err != nil {
		WriteError(c, err)
		return
	}

	c.JSON(http.StatusOK, dto.ToGuildResponses(guilds))
}

// Get handles GET /api/v1/guilds/:guildId.
func (ctrl *GuildController) Get(c *gin.Context) {
	userID, guildID, ok := ctrl.requireUserAndGuildID(c)
	if !ok {
		return
	}

	guild, err := ctrl.guilds.GetGuild(c.Request.Context(), userID, guildID)
	if err != nil {
		WriteError(c, err)
		return
	}

	c.JSON(http.StatusOK, dto.ToGuildResponse(*guild))
}

// Update handles PATCH /api/v1/guilds/:guildId.
func (ctrl *GuildController) Update(c *gin.Context) {
	userID, guildID, ok := ctrl.requireUserAndGuildID(c)
	if !ok {
		return
	}

	var req dto.UpdateGuildRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		WriteErrorStatus(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON request body")
		return
	}

	guild, err := ctrl.guilds.UpdateGuild(c.Request.Context(), userID, guildID, service.UpdateGuildInput{
		Name:       req.Name,
		IconURL:    req.IconURL.Value,
		IconURLSet: req.IconURL.Set,
	})
	if err != nil {
		WriteError(c, err)
		return
	}

	c.JSON(http.StatusOK, dto.ToGuildResponse(*guild))
}

// Delete handles DELETE /api/v1/guilds/:guildId.
func (ctrl *GuildController) Delete(c *gin.Context) {
	userID, guildID, ok := ctrl.requireUserAndGuildID(c)
	if !ok {
		return
	}

	if err := ctrl.guilds.DeleteGuild(c.Request.Context(), userID, guildID); err != nil {
		WriteError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// ListMembers handles GET /api/v1/guilds/:guildId/members.
func (ctrl *GuildController) ListMembers(c *gin.Context) {
	userID, guildID, ok := ctrl.requireUserAndGuildID(c)
	if !ok {
		return
	}

	members, err := ctrl.guilds.ListMembers(c.Request.Context(), userID, guildID)
	if err != nil {
		WriteError(c, err)
		return
	}

	c.JSON(http.StatusOK, dto.ToGuildMemberResponses(members))
}

// Leave handles DELETE /api/v1/guilds/:guildId/members/@me.
func (ctrl *GuildController) Leave(c *gin.Context) {
	userID, guildID, ok := ctrl.requireUserAndGuildID(c)
	if !ok {
		return
	}

	if err := ctrl.guilds.LeaveGuild(c.Request.Context(), userID, guildID); err != nil {
		WriteError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

func (ctrl *GuildController) requireUserAndGuildID(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	userID, ok := ctrl.extractUserID(c)
	if !ok {
		WriteErrorStatus(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return uuid.Nil, uuid.Nil, false
	}

	guildID, err := uuid.Parse(c.Param("guildId"))
	if err != nil {
		WriteErrorStatus(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid guildId parameter")
		return uuid.Nil, uuid.Nil, false
	}

	return userID, guildID, true
}
