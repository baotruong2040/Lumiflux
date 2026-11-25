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

// AuthUseCase defines the service operations needed by AuthController and UserController.
type AuthUseCase interface {
	Register(ctx context.Context, input service.RegisterInput) (*domain.AuthResult, error)
	Login(ctx context.Context, input service.LoginInput) (*domain.AuthResult, error)
	Refresh(ctx context.Context, refreshToken string) (*domain.AuthResult, error)
	Logout(ctx context.Context, refreshToken string) error
	GetMe(ctx context.Context, userID uuid.UUID) (*domain.User, error)
	UpdateMe(ctx context.Context, userID uuid.UUID, input service.UpdateProfileInput) (*domain.User, error)
}

// AuthController handles authentication HTTP endpoints.
type AuthController struct {
	auth AuthUseCase
}

// NewAuthController constructs an AuthController.
func NewAuthController(auth AuthUseCase) *AuthController {
	return &AuthController{auth: auth}
}

// Register handles POST /api/v1/auth/register.
func (ctrl *AuthController) Register(c *gin.Context) {
	var req dto.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		WriteErrorStatus(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON request body")
		return
	}

	res, err := ctrl.auth.Register(c.Request.Context(), service.RegisterInput{
		Username:    req.Username,
		Email:       req.Email,
		DisplayName: req.DisplayName,
		Password:    req.Password,
	})
	if err != nil {
		WriteError(c, err)
		return
	}

	c.JSON(http.StatusCreated, dto.ToAuthResponse(res))
}

// Login handles POST /api/v1/auth/login.
func (ctrl *AuthController) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		WriteErrorStatus(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON request body")
		return
	}

	res, err := ctrl.auth.Login(c.Request.Context(), service.LoginInput{
		Login:    req.Login,
		Password: req.Password,
	})
	if err != nil {
		WriteError(c, err)
		return
	}

	c.JSON(http.StatusOK, dto.ToAuthResponse(res))
}

// Refresh handles POST /api/v1/auth/refresh.
func (ctrl *AuthController) Refresh(c *gin.Context) {
	var req dto.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		WriteErrorStatus(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON request body")
		return
	}

	res, err := ctrl.auth.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		WriteError(c, err)
		return
	}

	c.JSON(http.StatusOK, dto.ToAuthResponse(res))
}

// Logout handles POST /api/v1/auth/logout.
func (ctrl *AuthController) Logout(c *gin.Context) {
	var req dto.LogoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		WriteErrorStatus(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON request body")
		return
	}

	if err := ctrl.auth.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		WriteError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}
