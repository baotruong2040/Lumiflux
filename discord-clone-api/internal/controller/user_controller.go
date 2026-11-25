package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/truong/discord-clone-api/internal/controller/dto"
	"github.com/truong/discord-clone-api/internal/service"
)

// UserIDExtractor retrieves the authenticated user ID from a Gin context.
type UserIDExtractor func(c *gin.Context) (uuid.UUID, bool)

// UserController handles authenticated user profile endpoints.
type UserController struct {
	auth          AuthUseCase
	extractUserID UserIDExtractor
}

// NewUserController constructs a UserController.
func NewUserController(auth AuthUseCase, extractUserID UserIDExtractor) *UserController {
	return &UserController{
		auth:          auth,
		extractUserID: extractUserID,
	}
}

// GetMe handles GET /api/v1/users/@me.
func (ctrl *UserController) GetMe(c *gin.Context) {
	userID, ok := ctrl.extractUserID(c)
	if !ok {
		WriteErrorStatus(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	user, err := ctrl.auth.GetMe(c.Request.Context(), userID)
	if err != nil {
		WriteError(c, err)
		return
	}

	c.JSON(http.StatusOK, dto.ToUserResponse(*user))
}

// UpdateMe handles PATCH /api/v1/users/@me.
func (ctrl *UserController) UpdateMe(c *gin.Context) {
	userID, ok := ctrl.extractUserID(c)
	if !ok {
		WriteErrorStatus(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req dto.UpdateMeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		WriteErrorStatus(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON request body")
		return
	}

	user, err := ctrl.auth.UpdateMe(c.Request.Context(), userID, service.UpdateProfileInput{
		DisplayName:  req.DisplayName,
		AvatarURL:    req.AvatarURL.Value,
		AvatarURLSet: req.AvatarURL.Set,
	})
	if err != nil {
		WriteError(c, err)
		return
	}

	c.JSON(http.StatusOK, dto.ToUserResponse(*user))
}
