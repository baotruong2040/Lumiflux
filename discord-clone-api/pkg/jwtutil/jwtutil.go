package jwtutil

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const defaultIssuer = "discord-clone-api"

var ErrInvalidToken = errors.New("invalid or expired token")

// AccessClaims defines the JWT claims embedded in access tokens.
type AccessClaims struct {
	jwt.RegisteredClaims
}

// Manager issues and validates JWT access tokens and opaque refresh tokens.
type Manager struct {
	secret    []byte
	accessTTL time.Duration
	issuer    string
}

// NewManager creates a token Manager with the provided HMAC secret and access token TTL.
func NewManager(secret string, accessTTL time.Duration) *Manager {
	return &Manager{
		secret:    []byte(secret),
		accessTTL: accessTTL,
		issuer:    defaultIssuer,
	}
}

// GenerateAccessToken signs a new HS256 access token for userID.
func (m *Manager) GenerateAccessToken(userID uuid.UUID) (string, error) {
	now := time.Now().UTC()
	jti, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate jti: %w", err)
	}

	claims := AccessClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   userID.String(),
			ID:        jti.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.accessTTL)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("sign access token: %w", err)
	}
	return signed, nil
}

// ParseAccessToken validates an HS256 access token and returns the embedded userID.
func (m *Manager) ParseAccessToken(tokenStr string) (uuid.UUID, error) {
	if tokenStr == "" {
		return uuid.Nil, ErrInvalidToken
	}

	var claims AccessClaims
	token, err := jwt.ParseWithClaims(tokenStr, &claims, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("%w: unexpected signing method %v", ErrInvalidToken, t.Header["alg"])
		}
		return m.secret, nil
	}, jwt.WithIssuer(m.issuer), jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !token.Valid {
		return uuid.Nil, ErrInvalidToken
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil, ErrInvalidToken
	}
	return userID, nil
}

// GenerateRefreshToken creates a cryptographically random refresh token and its SHA-256 hex digest.
func (m *Manager) GenerateRefreshToken() (raw string, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate refresh token bytes: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	return raw, m.HashRefreshToken(raw), nil
}

// HashRefreshToken returns the lowercase SHA-256 hex digest of a raw refresh token.
func (m *Manager) HashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
