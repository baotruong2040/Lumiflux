package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// UserRepository defines persistence operations for User entities.
type UserRepository interface {
	Create(ctx context.Context, user *User) error
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetByLogin(ctx context.Context, login string) (*User, error)
	Update(ctx context.Context, id uuid.UUID, params UpdateUserParams) (*User, error)
}

// TokenStore defines ephemeral storage operations for refresh token rotation and revocation.
type TokenStore interface {
	SaveRefreshToken(ctx context.Context, tokenHash string, userID uuid.UUID, ttl time.Duration) error
	ConsumeRefreshToken(ctx context.Context, tokenHash string) (uuid.UUID, error)
	DeleteRefreshToken(ctx context.Context, tokenHash string) error
}

// RateLimiter checks and increments sliding/fixed-window request counters.
type RateLimiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (allowed bool, retryAfter time.Duration, err error)
}

// PasswordHasher abstracts password hashing and verification.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(hash, password string) error
}

// TokenManager abstracts JWT access token and opaque refresh token operations.
type TokenManager interface {
	GenerateAccessToken(userID uuid.UUID) (string, error)
	ParseAccessToken(tokenStr string) (uuid.UUID, error)
	GenerateRefreshToken() (raw string, hash string, err error)
	HashRefreshToken(raw string) string
}

// HealthChecker verifies connectivity to an external dependency.
type HealthChecker interface {
	Ping(ctx context.Context) error
}
