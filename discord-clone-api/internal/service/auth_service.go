package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/truong/discord-clone-api/internal/domain"
	"github.com/truong/discord-clone-api/pkg/idgen"
)

var usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9._-]{3,32}$`)

// RegisterInput carries input fields for user registration.
type RegisterInput struct {
	Username    string
	Email       string
	DisplayName string
	Password    string
}

// LoginInput carries credentials for user login.
type LoginInput struct {
	Login    string
	Password string
}

// UpdateProfileInput carries mutable user profile fields.
type UpdateProfileInput struct {
	DisplayName  *string
	AvatarURL    *string
	AvatarURLSet bool
}

// AuthService orchestrates user registration, authentication, token rotation, and profile updates.
type AuthService struct {
	users      domain.UserRepository
	tokens     domain.TokenStore
	hasher     domain.PasswordHasher
	jwt        domain.TokenManager
	refreshTTL time.Duration
}

// NewAuthService constructs an AuthService with its domain dependencies.
func NewAuthService(
	users domain.UserRepository,
	tokens domain.TokenStore,
	hasher domain.PasswordHasher,
	jwt domain.TokenManager,
	refreshTTL time.Duration,
) *AuthService {
	return &AuthService{
		users:      users,
		tokens:     tokens,
		hasher:     hasher,
		jwt:        jwt,
		refreshTTL: refreshTTL,
	}
}

// Register validates user details, hashes the password, creates the account, and issues tokens.
func (s *AuthService) Register(ctx context.Context, input RegisterInput) (*domain.AuthResult, error) {
	username := strings.TrimSpace(input.Username)
	if !usernameRegex.MatchString(username) {
		return nil, domain.NewError(domain.ErrValidation, "VALIDATION_ERROR", "username must be 3-32 characters using letters, numbers, '.', '_', or '-'")
	}

	email := strings.TrimSpace(input.Email)
	if !isValidEmail(email) {
		return nil, domain.NewError(domain.ErrValidation, "VALIDATION_ERROR", "email must be a valid email address")
	}

	displayName, err := validateDisplayName(input.DisplayName)
	if err != nil {
		return nil, err
	}

	if err := validatePassword(input.Password); err != nil {
		return nil, err
	}

	id, err := idgen.NewUUIDv7()
	if err != nil {
		return nil, fmt.Errorf("generate user id: %w", err)
	}

	passwordHash, err := s.hasher.Hash(input.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user := &domain.User{
		ID:           id,
		Username:     username,
		Email:        email,
		DisplayName:  displayName,
		PasswordHash: passwordHash,
	}

	if err := s.users.Create(ctx, user); err != nil {
		return nil, err
	}

	return s.issueTokens(ctx, *user)
}

// Login authenticates a user by username or email and password, returning fresh tokens.
func (s *AuthService) Login(ctx context.Context, input LoginInput) (*domain.AuthResult, error) {
	login := strings.TrimSpace(input.Login)
	if login == "" || input.Password == "" {
		return nil, domain.NewError(domain.ErrInvalidCredentials, "INVALID_CREDENTIALS", "invalid username/email or password")
	}

	user, err := s.users.GetByLogin(ctx, login)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.NewError(domain.ErrInvalidCredentials, "INVALID_CREDENTIALS", "invalid username/email or password")
		}
		return nil, err
	}

	if err := s.hasher.Compare(user.PasswordHash, input.Password); err != nil {
		return nil, domain.NewError(domain.ErrInvalidCredentials, "INVALID_CREDENTIALS", "invalid username/email or password")
	}

	return s.issueTokens(ctx, *user)
}

// Refresh atomically consumes a valid refresh token and issues a rotated token pair.
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*domain.AuthResult, error) {
	raw := strings.TrimSpace(refreshToken)
	if raw == "" {
		return nil, domain.NewError(domain.ErrInvalidToken, "INVALID_REFRESH_TOKEN", "refresh token is required")
	}

	tokenHash := s.jwt.HashRefreshToken(raw)
	userID, err := s.tokens.ConsumeRefreshToken(ctx, tokenHash)
	if err != nil {
		return nil, err
	}

	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.NewError(domain.ErrInvalidToken, "INVALID_REFRESH_TOKEN", "refresh token user no longer exists")
		}
		return nil, err
	}

	return s.issueTokens(ctx, *user)
}

// Logout revokes the given refresh token from Redis.
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	raw := strings.TrimSpace(refreshToken)
	if raw == "" {
		return domain.NewError(domain.ErrValidation, "VALIDATION_ERROR", "refreshToken is required")
	}

	tokenHash := s.jwt.HashRefreshToken(raw)
	return s.tokens.DeleteRefreshToken(ctx, tokenHash)
}

// GetMe returns the profile for the authenticated userID.
func (s *AuthService) GetMe(ctx context.Context, userID uuid.UUID) (*domain.User, error) {
	return s.users.GetByID(ctx, userID)
}

// UpdateMe validates and persists profile updates for userID.
func (s *AuthService) UpdateMe(ctx context.Context, userID uuid.UUID, input UpdateProfileInput) (*domain.User, error) {
	params := domain.UpdateUserParams{
		AvatarURLSet: input.AvatarURLSet,
	}

	if input.DisplayName != nil {
		dn, err := validateDisplayName(*input.DisplayName)
		if err != nil {
			return nil, err
		}
		params.DisplayName = &dn
	}

	if input.AvatarURLSet && input.AvatarURL != nil {
		trimmed := strings.TrimSpace(*input.AvatarURL)
		if trimmed == "" {
			params.AvatarURL = nil
		} else {
			if len(trimmed) > 2048 || !isValidHTTPURL(trimmed) {
				return nil, domain.NewError(domain.ErrValidation, "VALIDATION_ERROR", "avatarUrl must be a valid http or https URL")
			}
			params.AvatarURL = &trimmed
		}
	}

	return s.users.Update(ctx, userID, params)
}

func (s *AuthService) issueTokens(ctx context.Context, user domain.User) (*domain.AuthResult, error) {
	accessToken, err := s.jwt.GenerateAccessToken(user.ID)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	rawRefresh, refreshHash, err := s.jwt.GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	if err := s.tokens.SaveRefreshToken(ctx, refreshHash, user.ID, s.refreshTTL); err != nil {
		return nil, fmt.Errorf("store refresh token: %w", err)
	}

	return &domain.AuthResult{
		AccessToken:  accessToken,
		RefreshToken: rawRefresh,
		User:         user,
	}, nil
}

func validateDisplayName(raw string) (string, error) {
	dn := strings.TrimSpace(raw)
	n := utf8.RuneCountInString(dn)
	if n < 1 || n > 64 {
		return "", domain.NewError(domain.ErrValidation, "VALIDATION_ERROR", "displayName must be between 1 and 64 characters")
	}
	return dn, nil
}

func validatePassword(password string) error {
	if len(password) < 8 {
		return domain.NewError(domain.ErrValidation, "VALIDATION_ERROR", "password must be at least 8 characters")
	}
	if len([]byte(password)) > 72 {
		return domain.NewError(domain.ErrValidation, "VALIDATION_ERROR", "password must not exceed 72 bytes")
	}
	return nil
}

func isValidEmail(email string) bool {
	if email == "" || len(email) > 254 || strings.ContainsAny(email, " \t\r\n") {
		return false
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email {
		return false
	}
	_, domainPart, ok := strings.Cut(email, "@")
	if !ok || domainPart == "" || !strings.Contains(domainPart, ".") {
		return false
	}
	return true
}

func isValidHTTPURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
