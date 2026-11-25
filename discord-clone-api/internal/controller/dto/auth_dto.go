package dto

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/truong/discord-clone-api/internal/domain"
)

// RegisterRequest represents the JSON body for POST /api/v1/auth/register.
type RegisterRequest struct {
	Username    string `json:"username"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Password    string `json:"password"`
}

// LoginRequest represents the JSON body for POST /api/v1/auth/login.
type LoginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

// RefreshRequest represents the JSON body for POST /api/v1/auth/refresh.
type RefreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

// LogoutRequest represents the JSON body for POST /api/v1/auth/logout.
type LogoutRequest struct {
	RefreshToken string `json:"refreshToken"`
}

// NullableString distinguishes omitted JSON fields from explicit null or string values.
type NullableString struct {
	Set   bool
	Value *string
}

// UnmarshalJSON tracks whether the field was explicitly present in JSON.
func (n *NullableString) UnmarshalJSON(data []byte) error {
	n.Set = true
	trimmed := bytes.TrimSpace(data)
	if bytes.Equal(trimmed, []byte("null")) {
		n.Value = nil
		return nil
	}
	var s string
	if err := json.Unmarshal(trimmed, &s); err != nil {
		return err
	}
	n.Value = &s
	return nil
}

// UpdateMeRequest represents the JSON body for PATCH /api/v1/users/@me.
type UpdateMeRequest struct {
	DisplayName *string        `json:"displayName"`
	AvatarURL   NullableString `json:"avatarUrl"`
}

// UserResponse matches the User schema in api/openapi.yaml.
type UserResponse struct {
	ID          uuid.UUID `json:"id"`
	Username    string    `json:"username"`
	Email       string    `json:"email"`
	DisplayName string    `json:"displayName"`
	AvatarURL   *string   `json:"avatarUrl"`
	CreatedAt   time.Time `json:"createdAt"`
}

// AuthResponse matches the AuthResponse schema in api/openapi.yaml.
type AuthResponse struct {
	AccessToken  string       `json:"accessToken"`
	RefreshToken string       `json:"refreshToken"`
	User         UserResponse `json:"user"`
}

// ToUserResponse converts a domain User into a UserResponse DTO.
func ToUserResponse(u domain.User) UserResponse {
	return UserResponse{
		ID:          u.ID,
		Username:    u.Username,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		AvatarURL:   u.AvatarURL,
		CreatedAt:   u.CreatedAt,
	}
}

// ToAuthResponse converts a domain AuthResult into an AuthResponse DTO.
func ToAuthResponse(res *domain.AuthResult) AuthResponse {
	return AuthResponse{
		AccessToken:  res.AccessToken,
		RefreshToken: res.RefreshToken,
		User:         ToUserResponse(res.User),
	}
}
