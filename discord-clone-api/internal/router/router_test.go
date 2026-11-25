package router

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/truong/discord-clone-api/internal/config"
	"github.com/truong/discord-clone-api/internal/controller"
	"github.com/truong/discord-clone-api/internal/controller/dto"
	"github.com/truong/discord-clone-api/internal/domain"
	"github.com/truong/discord-clone-api/internal/router/middleware"
	"github.com/truong/discord-clone-api/internal/service"
	"github.com/truong/discord-clone-api/pkg/hasher"
	"github.com/truong/discord-clone-api/pkg/jwtutil"
	"github.com/truong/discord-clone-api/pkg/logger"
	"golang.org/x/crypto/bcrypt"
)

type stubHealthChecker struct {
	err error
}

func (s *stubHealthChecker) Ping(_ context.Context) error {
	return s.err
}

type memoryUserRepo struct {
	mu    sync.RWMutex
	users map[uuid.UUID]domain.User
}

func newMemoryUserRepo() *memoryUserRepo {
	return &memoryUserRepo{users: make(map[uuid.UUID]domain.User)}
}

func (r *memoryUserRepo) Create(_ context.Context, user *domain.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.users {
		if strings.EqualFold(existing.Username, user.Username) {
			return domain.NewError(domain.ErrConflict, "USERNAME_TAKEN", "username is already taken")
		}
		if strings.EqualFold(existing.Email, user.Email) {
			return domain.NewError(domain.ErrConflict, "EMAIL_TAKEN", "email is already registered")
		}
	}
	user.CreatedAt = time.Now().UTC()
	r.users[user.ID] = *user
	return nil
}

func (r *memoryUserRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u, ok := r.users[id]
	if !ok {
		return nil, domain.NewError(domain.ErrNotFound, "USER_NOT_FOUND", "user not found")
	}
	cp := u
	return &cp, nil
}

func (r *memoryUserRepo) GetByLogin(_ context.Context, login string) (*domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, u := range r.users {
		if strings.EqualFold(u.Username, login) || strings.EqualFold(u.Email, login) {
			cp := u
			return &cp, nil
		}
	}
	return nil, domain.NewError(domain.ErrNotFound, "USER_NOT_FOUND", "user not found")
}

func (r *memoryUserRepo) Update(_ context.Context, id uuid.UUID, params domain.UpdateUserParams) (*domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.users[id]
	if !ok {
		return nil, domain.NewError(domain.ErrNotFound, "USER_NOT_FOUND", "user not found")
	}
	if params.DisplayName != nil {
		u.DisplayName = *params.DisplayName
	}
	if params.AvatarURLSet {
		u.AvatarURL = params.AvatarURL
	}
	r.users[id] = u
	cp := u
	return &cp, nil
}

type memoryTokenStore struct {
	mu     sync.Mutex
	tokens map[string]uuid.UUID
}

func newMemoryTokenStore() *memoryTokenStore {
	return &memoryTokenStore{tokens: make(map[string]uuid.UUID)}
}

func (s *memoryTokenStore) SaveRefreshToken(_ context.Context, hash string, userID uuid.UUID, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[hash] = userID
	return nil
}

func (s *memoryTokenStore) ConsumeRefreshToken(_ context.Context, hash string) (uuid.UUID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.tokens[hash]
	if !ok {
		return uuid.Nil, domain.NewError(domain.ErrInvalidToken, "INVALID_REFRESH_TOKEN", "refresh token is invalid or expired")
	}
	delete(s.tokens, hash)
	return id, nil
}

func (s *memoryTokenStore) DeleteRefreshToken(_ context.Context, hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, hash)
	return nil
}

type memoryRateLimiter struct {
	mu     sync.Mutex
	counts map[string]int
}

func newMemoryRateLimiter() *memoryRateLimiter {
	return &memoryRateLimiter{counts: make(map[string]int)}
}

func (l *memoryRateLimiter) Allow(_ context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.counts[key]++
	if l.counts[key] > limit {
		return false, window, nil
	}
	return true, 0, nil
}

func newTestRouter(pgErr, redisErr error) (*gin.Engine, *jwtutil.Manager) {
	cfg := &config.Config{
		AppEnv:             "test",
		Port:               "8080",
		LogLevel:           "error",
		JWTSecret:          "router-test-secret-key-at-least-32-bytes!",
		AccessTokenTTL:     15 * time.Minute,
		RefreshTokenTTL:    7 * 24 * time.Hour,
		BcryptCost:         bcrypt.MinCost,
		CORSAllowedOrigins: []string{"http://localhost:5173", "tauri://localhost", "http://tauri.localhost", "https://tauri.localhost"},
		LoginRateLimit:     10,
		LoginRateWindow:    time.Minute,
	}

	log := logger.NewWithWriter(io.Discard, "error")
	jwtMgr := jwtutil.NewManager(cfg.JWTSecret, cfg.AccessTokenTTL)
	authSvc := service.NewAuthService(
		newMemoryUserRepo(),
		newMemoryTokenStore(),
		hasher.NewBcryptHasher(cfg.BcryptCost),
		jwtMgr,
		cfg.RefreshTokenTTL,
	)

	r := New(Dependencies{
		Config:           cfg,
		Logger:           log,
		TokenManager:     jwtMgr,
		RateLimiter:      newMemoryRateLimiter(),
		HealthController: controller.NewHealthController(&stubHealthChecker{err: pgErr}, &stubHealthChecker{err: redisErr}, log),
		AuthController:   controller.NewAuthController(authSvc),
		UserController:   controller.NewUserController(authSvc, middleware.GetUserID),
	})

	return r, jwtMgr
}

func doJSONRequest(t *testing.T, r http.Handler, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.RemoteAddr = "198.51.100.10:12345"
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestRouter_HealthAndReadyz(t *testing.T) {
	r, _ := newTestRouter(nil, nil)

	rec := doJSONRequest(t, r, http.MethodGet, "/healthz", nil, nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("X-Request-ID"))

	rec = doJSONRequest(t, r, http.MethodGet, "/readyz", nil, map[string]string{"X-Request-ID": "custom-trace-1"})
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "custom-trace-1", rec.Header().Get("X-Request-ID"))

	rPgDown, _ := newTestRouter(errors.New("pg down"), nil)
	rec = doJSONRequest(t, rPgDown, http.MethodGet, "/readyz", nil, nil)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	rRedisDown, _ := newTestRouter(nil, errors.New("redis down"))
	rec = doJSONRequest(t, rRedisDown, http.MethodGet, "/readyz", nil, nil)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestRouter_CORSAndRecover(t *testing.T) {
	r, _ := newTestRouter(nil, nil)
	r.GET("/panic-test", func(_ *gin.Context) {
		panic("boom")
	})

	for _, origin := range []string{"http://localhost:5173", "tauri://localhost", "http://tauri.localhost", "https://tauri.localhost"} {
		rec := doJSONRequest(t, r, http.MethodOptions, "/healthz", nil, map[string]string{"Origin": origin})
		assert.Equal(t, http.StatusNoContent, rec.Code)
		assert.Equal(t, origin, rec.Header().Get("Access-Control-Allow-Origin"))
	}

	recBlocked := doJSONRequest(t, r, http.MethodGet, "/healthz", nil, map[string]string{"Origin": "https://evil.example.com"})
	assert.Equal(t, http.StatusOK, recBlocked.Code)
	assert.Empty(t, recBlocked.Header().Get("Access-Control-Allow-Origin"))

	recPanic := doJSONRequest(t, r, http.MethodGet, "/panic-test", nil, nil)
	assert.Equal(t, http.StatusInternalServerError, recPanic.Code)
	var errResp controller.ErrorResponse
	require.NoError(t, json.Unmarshal(recPanic.Body.Bytes(), &errResp))
	assert.Equal(t, "INTERNAL_ERROR", errResp.Error.Code)
}

func TestRouter_AuthFlowAndRateLimit(t *testing.T) {
	r, _ := newTestRouter(nil, nil)

	// Unauthenticated @me returns 401
	rec := doJSONRequest(t, r, http.MethodGet, "/api/v1/users/@me", nil, nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	// Register -> 201
	rec = doJSONRequest(t, r, http.MethodPost, "/api/v1/auth/register", dto.RegisterRequest{
		Username:    "truong",
		Email:       "truong@example.com",
		DisplayName: "Truong",
		Password:    "Password123!",
	}, nil)
	require.Equal(t, http.StatusCreated, rec.Code)

	var regResp dto.AuthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &regResp))
	assert.NotEmpty(t, regResp.AccessToken)
	assert.NotEmpty(t, regResp.RefreshToken)
	assert.Equal(t, "truong", regResp.User.Username)

	// Duplicate registration (case-insensitive) -> 409
	rec = doJSONRequest(t, r, http.MethodPost, "/api/v1/auth/register", dto.RegisterRequest{
		Username:    "TRUONG",
		Email:       "other@example.com",
		DisplayName: "Dup",
		Password:    "Password123!",
	}, nil)
	assert.Equal(t, http.StatusConflict, rec.Code)

	// GET /api/v1/users/@me -> 200
	rec = doJSONRequest(t, r, http.MethodGet, "/api/v1/users/@me", nil, map[string]string{
		"Authorization": "Bearer " + regResp.AccessToken,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// PATCH /api/v1/users/@me -> set displayName & avatarUrl, then clear avatarUrl to null
	rec = doJSONRequest(t, r, http.MethodPatch, "/api/v1/users/@me", map[string]any{
		"displayName": "Truong Updated",
		"avatarUrl":   "https://cdn.example.com/a.png",
	}, map[string]string{"Authorization": "Bearer " + regResp.AccessToken})
	require.Equal(t, http.StatusOK, rec.Code)
	var updated dto.UserResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updated))
	assert.Equal(t, "Truong Updated", updated.DisplayName)
	require.NotNil(t, updated.AvatarURL)
	assert.Equal(t, "https://cdn.example.com/a.png", *updated.AvatarURL)

	rec = doJSONRequest(t, r, http.MethodPatch, "/api/v1/users/@me", map[string]any{
		"avatarUrl": nil,
	}, map[string]string{"Authorization": "Bearer " + regResp.AccessToken})
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updated))
	assert.Nil(t, updated.AvatarURL)

	// Refresh rotation -> 200, reusing old token -> 401
	rec = doJSONRequest(t, r, http.MethodPost, "/api/v1/auth/refresh", dto.RefreshRequest{
		RefreshToken: regResp.RefreshToken,
	}, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var refResp dto.AuthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &refResp))
	assert.NotEqual(t, regResp.RefreshToken, refResp.RefreshToken)

	rec = doJSONRequest(t, r, http.MethodPost, "/api/v1/auth/refresh", dto.RefreshRequest{
		RefreshToken: regResp.RefreshToken,
	}, nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	// Logout -> 204, reusing revoked token -> 401
	rec = doJSONRequest(t, r, http.MethodPost, "/api/v1/auth/logout", dto.LogoutRequest{
		RefreshToken: refResp.RefreshToken,
	}, map[string]string{"Authorization": "Bearer " + refResp.AccessToken})
	assert.Equal(t, http.StatusNoContent, rec.Code)

	rec = doJSONRequest(t, r, http.MethodPost, "/api/v1/auth/refresh", dto.RefreshRequest{
		RefreshToken: refResp.RefreshToken,
	}, nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	// Login rate limit: 10 failed attempts -> 401, 11th -> 429 RATE_LIMITED
	for i := range 10 {
		rec = doJSONRequest(t, r, http.MethodPost, "/api/v1/auth/login", dto.LoginRequest{
			Login:    "truong",
			Password: "wrong-password",
		}, nil)
		assert.Equal(t, http.StatusUnauthorized, rec.Code, "attempt %d should be 401", i+1)
	}
	rec = doJSONRequest(t, r, http.MethodPost, "/api/v1/auth/login", dto.LoginRequest{
		Login:    "truong",
		Password: "wrong-password",
	}, nil)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
	var rlErr controller.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &rlErr))
	assert.Equal(t, "RATE_LIMITED", rlErr.Error.Code)
}
