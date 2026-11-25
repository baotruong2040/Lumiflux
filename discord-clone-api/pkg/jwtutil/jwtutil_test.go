package jwtutil

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_AccessTokenLifecycle(t *testing.T) {
	mgr := NewManager("test-secret-at-least-32-bytes-long!!", 15*time.Minute)
	userID := uuid.Must(uuid.NewV7())

	tokenStr, err := mgr.GenerateAccessToken(userID)
	require.NoError(t, err)
	assert.NotEmpty(t, tokenStr)

	parsedID, err := mgr.ParseAccessToken(tokenStr)
	require.NoError(t, err)
	assert.Equal(t, userID, parsedID)

	// Wrong secret rejected.
	otherMgr := NewManager("different-secret-at-least-32-bytes!", 15*time.Minute)
	_, err = otherMgr.ParseAccessToken(tokenStr)
	assert.ErrorIs(t, err, ErrInvalidToken)

	// Expired token rejected.
	expiredMgr := NewManager("test-secret-at-least-32-bytes-long!!", -1*time.Minute)
	expiredToken, err := expiredMgr.GenerateAccessToken(userID)
	require.NoError(t, err)
	_, err = mgr.ParseAccessToken(expiredToken)
	assert.ErrorIs(t, err, ErrInvalidToken)

	// Unexpected alg rejected (e.g. HS512).
	hs512 := jwt.NewWithClaims(jwt.SigningMethodHS512, AccessClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "discord-clone-api",
			Subject:   userID.String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
	})
	hs512Str, err := hs512.SignedString([]byte("test-secret-at-least-32-bytes-long!!"))
	require.NoError(t, err)
	_, err = mgr.ParseAccessToken(hs512Str)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestManager_RefreshTokenUniquenessAndHash(t *testing.T) {
	mgr := NewManager("test-secret", 15*time.Minute)

	raw1, hash1, err := mgr.GenerateRefreshToken()
	require.NoError(t, err)
	raw2, hash2, err := mgr.GenerateRefreshToken()
	require.NoError(t, err)

	assert.NotEmpty(t, raw1)
	assert.Len(t, hash1, 64)
	assert.NotEqual(t, raw1, raw2)
	assert.NotEqual(t, hash1, hash2)
	assert.Equal(t, hash1, mgr.HashRefreshToken(raw1))
}
