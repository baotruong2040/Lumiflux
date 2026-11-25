package domain

import (
	"time"

	"github.com/google/uuid"
)

// User represents a registered account in the system.
type User struct {
	ID           uuid.UUID
	Username     string
	Email        string
	DisplayName  string
	PasswordHash string
	AvatarURL    *string
	CreatedAt    time.Time
}

// UpdateUserParams specifies mutable profile fields for a partial user update.
type UpdateUserParams struct {
	DisplayName  *string
	AvatarURL    *string
	AvatarURLSet bool
}

// AuthResult contains newly issued authentication tokens and the authenticated user.
type AuthResult struct {
	AccessToken  string
	RefreshToken string
	User         User
}
