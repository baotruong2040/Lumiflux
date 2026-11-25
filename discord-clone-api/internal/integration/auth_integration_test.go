package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/truong/discord-clone-api/internal/config"
	"github.com/truong/discord-clone-api/internal/controller"
	"github.com/truong/discord-clone-api/internal/controller/dto"
	"github.com/truong/discord-clone-api/internal/domain"
	"github.com/truong/discord-clone-api/internal/repository/postgres"
	"github.com/truong/discord-clone-api/internal/repository/redisstore"
	"github.com/truong/discord-clone-api/internal/router"
	"github.com/truong/discord-clone-api/internal/router/middleware"
	"github.com/truong/discord-clone-api/internal/service"
	"github.com/truong/discord-clone-api/pkg/hasher"
	"github.com/truong/discord-clone-api/pkg/jwtutil"
	"github.com/truong/discord-clone-api/pkg/logger"
	"golang.org/x/crypto/bcrypt"
)

type subEvent struct {
	userID  uuid.UUID
	guildID uuid.UUID
}

type recordingSubManager struct {
	mu           sync.Mutex
	subscribed   []subEvent
	unsubscribed []subEvent
}

func (m *recordingSubManager) Subscribe(_ context.Context, userID, guildID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subscribed = append(m.subscribed, subEvent{userID: userID, guildID: guildID})
}

func (m *recordingSubManager) Unsubscribe(_ context.Context, userID, guildID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.unsubscribed = append(m.unsubscribed, subEvent{userID: userID, guildID: guildID})
}

func (m *recordingSubManager) snapshot() (subscribed []subEvent, unsubscribed []subEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	subscribed = append([]subEvent(nil), m.subscribed...)
	unsubscribed = append([]subEvent(nil), m.unsubscribed...)
	return subscribed, unsubscribed
}

type testEnv struct {
	cfg             *config.Config
	pool            *pgxpool.Pool
	redis           *redis.Client
	jwtMgr          *jwtutil.Manager
	subs            *recordingSubManager
	handler         http.Handler
	createdIDs      []uuid.UUID
	createdGuildIDs []uuid.UUID
	redisClean      []string
}

func setupIntegrationEnv(t *testing.T) *testEnv {
	t.Helper()
	if os.Getenv("RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("set RUN_INTEGRATION_TESTS=1 to run integration tests")
	}

	cfg, err := config.Load()
	require.NoError(t, err)
	cfg.AppEnv = "test"

	ctx := context.Background()
	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	require.NoError(t, err)

	redisClient, err := redisstore.NewClient(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	require.NoError(t, err)

	log := logger.NewWithWriter(io.Discard, "error")
	jwtMgr := jwtutil.NewManager(cfg.JWTSecret, cfg.AccessTokenTTL)
	userRepo := postgres.NewUserRepository(pool)
	guildRepo := postgres.NewGuildRepository(pool)
	channelRepo := postgres.NewChannelRepository(pool)
	inviteRepo := postgres.NewInviteRepository(pool)
	subs := &recordingSubManager{}

	authSvc := service.NewAuthService(
		userRepo,
		redisstore.NewTokenStore(redisClient),
		hasher.NewBcryptHasher(bcrypt.MinCost),
		jwtMgr,
		cfg.RefreshTokenTTL,
	)
	guildSvc := service.NewGuildService(guildRepo, redisstore.NewPresenceStore(redisClient), subs)
	inviteSvc := service.NewInviteService(guildRepo, inviteRepo, subs)
	channelSvc := service.NewChannelService(guildRepo, channelRepo)

	engine := router.New(router.Dependencies{
		Config:            cfg,
		Logger:            log,
		TokenManager:      jwtMgr,
		RateLimiter:       redisstore.NewRateLimiter(redisClient),
		HealthController:  controller.NewHealthController(postgres.NewHealthChecker(pool), redisstore.NewHealthChecker(redisClient), log),
		AuthController:    controller.NewAuthController(authSvc),
		UserController:    controller.NewUserController(authSvc, middleware.GetUserID),
		GuildController:   controller.NewGuildController(guildSvc, middleware.GetUserID),
		InviteController:  controller.NewInviteController(inviteSvc, middleware.GetUserID),
		ChannelController: controller.NewChannelController(channelSvc, middleware.GetUserID),
	})

	env := &testEnv{
		cfg:     cfg,
		pool:    pool,
		redis:   redisClient,
		jwtMgr:  jwtMgr,
		subs:    subs,
		handler: engine,
	}

	t.Cleanup(func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, gid := range env.createdGuildIDs {
			_, _ = env.pool.Exec(cleanCtx, `DELETE FROM guilds WHERE id = $1`, gid)
		}
		for _, id := range env.createdIDs {
			_, _ = env.pool.Exec(cleanCtx, `DELETE FROM users WHERE id = $1`, id)
		}
		if len(env.redisClean) > 0 {
			_ = env.redis.Del(cleanCtx, env.redisClean...).Err()
		}
		_ = env.redis.Close()
		env.pool.Close()
	})

	return env
}

func (e *testEnv) trackUser(id uuid.UUID) {
	e.createdIDs = append(e.createdIDs, id)
}

func (e *testEnv) trackGuild(id uuid.UUID) {
	e.createdGuildIDs = append(e.createdGuildIDs, id)
}

var _ domain.GuildSubscriptionManager = (*recordingSubManager)(nil)

func (e *testEnv) trackRedisKey(key string) {
	e.redisClean = append(e.redisClean, key)
}

func (e *testEnv) do(t *testing.T, method, path string, body any, headers map[string]string, remoteAddr string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	} else {
		req.RemoteAddr = "192.0.2.10:12345"
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

func uniqueSuffix() string {
	return strings.ReplaceAll(uuid.Must(uuid.NewV7()).String(), "-", "")[:12]
}

func TestIntegration_Readyz(t *testing.T) {
	env := setupIntegrationEnv(t)

	rec := env.do(t, http.MethodGet, "/healthz", nil, nil, "")
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = env.do(t, http.MethodGet, "/readyz", nil, nil, "")
	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "ready", body["status"])
	checks, ok := body["checks"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "ok", checks["postgres"])
	assert.Equal(t, "ok", checks["redis"])
}

func TestIntegration_TC_AUTH_01_Register(t *testing.T) {
	env := setupIntegrationEnv(t)
	suf := uniqueSuffix()

	rec := env.do(t, http.MethodPost, "/api/v1/auth/register", dto.RegisterRequest{
		Username:    "tc01_" + suf,
		Email:       fmt.Sprintf("tc01_%s@example.com", suf),
		DisplayName: "TC Auth 01",
		Password:    "StrongPass123!",
	}, nil, "")
	require.Equal(t, http.StatusCreated, rec.Code)

	var res dto.AuthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	env.trackUser(res.User.ID)
	env.trackRedisKey("refresh:" + env.jwtMgr.HashRefreshToken(res.RefreshToken))

	assert.Equal(t, uuid.Version(7), res.User.ID.Version())

	var dbHash string
	err := env.pool.QueryRow(context.Background(), `SELECT password_hash FROM users WHERE id = $1`, res.User.ID).Scan(&dbHash)
	require.NoError(t, err)
	assert.NotEqual(t, "StrongPass123!", dbHash)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(dbHash), []byte("StrongPass123!")))
}

func TestIntegration_TC_AUTH_02_DuplicateCaseInsensitive(t *testing.T) {
	env := setupIntegrationEnv(t)
	suf := uniqueSuffix()
	username := "TC02_" + suf
	email := fmt.Sprintf("TC02_%s@Example.com", suf)

	rec := env.do(t, http.MethodPost, "/api/v1/auth/register", dto.RegisterRequest{
		Username:    username,
		Email:       email,
		DisplayName: "TC Auth 02",
		Password:    "StrongPass123!",
	}, nil, "")
	require.Equal(t, http.StatusCreated, rec.Code)

	var res dto.AuthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	env.trackUser(res.User.ID)
	env.trackRedisKey("refresh:" + env.jwtMgr.HashRefreshToken(res.RefreshToken))

	// Duplicate username (lowercase) -> 409
	rec = env.do(t, http.MethodPost, "/api/v1/auth/register", dto.RegisterRequest{
		Username:    strings.ToLower(username),
		Email:       fmt.Sprintf("other_%s@example.com", suf),
		DisplayName: "Dup User",
		Password:    "StrongPass123!",
	}, nil, "")
	assert.Equal(t, http.StatusConflict, rec.Code)

	// Duplicate email (lowercase) -> 409
	rec = env.do(t, http.MethodPost, "/api/v1/auth/register", dto.RegisterRequest{
		Username:    "other_" + suf,
		Email:       strings.ToLower(email),
		DisplayName: "Dup Email",
		Password:    "StrongPass123!",
	}, nil, "")
	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestIntegration_TC_AUTH_03_LoginAndRedisHash(t *testing.T) {
	env := setupIntegrationEnv(t)
	suf := uniqueSuffix()
	username := "TC03_" + suf
	email := fmt.Sprintf("TC03_%s@Example.com", suf)

	rec := env.do(t, http.MethodPost, "/api/v1/auth/register", dto.RegisterRequest{
		Username:    username,
		Email:       email,
		DisplayName: "TC Auth 03",
		Password:    "StrongPass123!",
	}, nil, "")
	require.Equal(t, http.StatusCreated, rec.Code)
	var regRes dto.AuthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &regRes))
	env.trackUser(regRes.User.ID)
	env.trackRedisKey("refresh:" + env.jwtMgr.HashRefreshToken(regRes.RefreshToken))

	rec = env.do(t, http.MethodPost, "/api/v1/auth/login", dto.LoginRequest{
		Login:    strings.ToLower(email),
		Password: "StrongPass123!",
	}, nil, "")
	require.Equal(t, http.StatusOK, rec.Code)

	var loginRes dto.AuthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &loginRes))
	refreshKey := "refresh:" + env.jwtMgr.HashRefreshToken(loginRes.RefreshToken)
	env.trackRedisKey(refreshKey)

	// Verify AccessToken TTL = 15m
	var claims jwtutil.AccessClaims
	_, err := jwt.ParseWithClaims(loginRes.AccessToken, &claims, func(_ *jwt.Token) (any, error) {
		return []byte(env.cfg.JWTSecret), nil
	})
	require.NoError(t, err)
	assert.Equal(t, 15*time.Minute, claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time))

	// Verify Redis refresh:{hash} stores userID with ~7 day TTL
	val, err := env.redis.Get(context.Background(), refreshKey).Result()
	require.NoError(t, err)
	assert.Equal(t, regRes.User.ID.String(), val)

	ttl, err := env.redis.TTL(context.Background(), refreshKey).Result()
	require.NoError(t, err)
	assert.Greater(t, ttl, 6*24*time.Hour+23*time.Hour)
	assert.LessOrEqual(t, ttl, 7*24*time.Hour)
}

func TestIntegration_TC_AUTH_04_RefreshRotationAndLogout(t *testing.T) {
	env := setupIntegrationEnv(t)
	suf := uniqueSuffix()

	rec := env.do(t, http.MethodPost, "/api/v1/auth/register", dto.RegisterRequest{
		Username:    "tc04_" + suf,
		Email:       fmt.Sprintf("tc04_%s@example.com", suf),
		DisplayName: "TC Auth 04",
		Password:    "StrongPass123!",
	}, nil, "")
	require.Equal(t, http.StatusCreated, rec.Code)

	var regRes dto.AuthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &regRes))
	env.trackUser(regRes.User.ID)
	oldKey := "refresh:" + env.jwtMgr.HashRefreshToken(regRes.RefreshToken)
	env.trackRedisKey(oldKey)

	// Rotate refresh token
	rec = env.do(t, http.MethodPost, "/api/v1/auth/refresh", dto.RefreshRequest{
		RefreshToken: regRes.RefreshToken,
	}, nil, "")
	require.Equal(t, http.StatusOK, rec.Code)

	var rotRes dto.AuthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &rotRes))
	newKey := "refresh:" + env.jwtMgr.HashRefreshToken(rotRes.RefreshToken)
	env.trackRedisKey(newKey)
	assert.NotEqual(t, regRes.RefreshToken, rotRes.RefreshToken)

	// Reusing old refresh token immediately returns 401
	rec = env.do(t, http.MethodPost, "/api/v1/auth/refresh", dto.RefreshRequest{
		RefreshToken: regRes.RefreshToken,
	}, nil, "")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	// GET & PATCH /api/v1/users/@me
	authHdr := map[string]string{"Authorization": "Bearer " + rotRes.AccessToken}
	rec = env.do(t, http.MethodGet, "/api/v1/users/@me", nil, authHdr, "")
	require.Equal(t, http.StatusOK, rec.Code)

	rec = env.do(t, http.MethodPatch, "/api/v1/users/@me", map[string]any{
		"displayName": "TC04 Updated",
		"avatarUrl":   "https://cdn.example.com/tc04.png",
	}, authHdr, "")
	require.Equal(t, http.StatusOK, rec.Code)

	var me dto.UserResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &me))
	assert.Equal(t, "TC04 Updated", me.DisplayName)
	require.NotNil(t, me.AvatarURL)
	assert.Equal(t, "https://cdn.example.com/tc04.png", *me.AvatarURL)

	// Logout -> 204 and deletes Redis refresh key
	rec = env.do(t, http.MethodPost, "/api/v1/auth/logout", dto.LogoutRequest{
		RefreshToken: rotRes.RefreshToken,
	}, authHdr, "")
	assert.Equal(t, http.StatusNoContent, rec.Code)

	exists, err := env.redis.Exists(context.Background(), newKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), exists)

	rec = env.do(t, http.MethodPost, "/api/v1/auth/refresh", dto.RefreshRequest{
		RefreshToken: rotRes.RefreshToken,
	}, nil, "")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestIntegration_TC_AUTH_05_ExpiredAccessTokenRefreshFlow(t *testing.T) {
	env := setupIntegrationEnv(t)
	suf := uniqueSuffix()

	rec := env.do(t, http.MethodPost, "/api/v1/auth/register", dto.RegisterRequest{
		Username:    "tc05_" + suf,
		Email:       fmt.Sprintf("tc05_%s@example.com", suf),
		DisplayName: "TC Auth 05",
		Password:    "StrongPass123!",
	}, nil, "")
	require.Equal(t, http.StatusCreated, rec.Code)

	var regRes dto.AuthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &regRes))
	env.trackUser(regRes.User.ID)
	env.trackRedisKey("refresh:" + env.jwtMgr.HashRefreshToken(regRes.RefreshToken))

	// Mint an already-expired access token with the same secret
	expiredMgr := jwtutil.NewManager(env.cfg.JWTSecret, -1*time.Minute)
	expiredAccess, err := expiredMgr.GenerateAccessToken(regRes.User.ID)
	require.NoError(t, err)

	// Request with expired access token fails with 401
	rec = env.do(t, http.MethodGet, "/api/v1/users/@me", nil, map[string]string{
		"Authorization": "Bearer " + expiredAccess,
	}, "")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	// Client calls /auth/refresh and retries original request with new accessToken -> 200
	rec = env.do(t, http.MethodPost, "/api/v1/auth/refresh", dto.RefreshRequest{
		RefreshToken: regRes.RefreshToken,
	}, nil, "")
	require.Equal(t, http.StatusOK, rec.Code)

	var refRes dto.AuthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &refRes))
	env.trackRedisKey("refresh:" + env.jwtMgr.HashRefreshToken(refRes.RefreshToken))

	rec = env.do(t, http.MethodGet, "/api/v1/users/@me", nil, map[string]string{
		"Authorization": "Bearer " + refRes.AccessToken,
	}, "")
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestIntegration_TC_AUTH_06_LoginRateLimit(t *testing.T) {
	env := setupIntegrationEnv(t)
	suf := uniqueSuffix()
	testIP := "203.0.113." + fmt.Sprintf("%d", 100+(time.Now().UnixNano()%100))
	remoteAddr := testIP + ":54321"
	rlKey := "rl:login:" + testIP
	env.trackRedisKey(rlKey)
	_ = env.redis.Del(context.Background(), rlKey).Err()

	rec := env.do(t, http.MethodPost, "/api/v1/auth/register", dto.RegisterRequest{
		Username:    "tc06_" + suf,
		Email:       fmt.Sprintf("tc06_%s@example.com", suf),
		DisplayName: "TC Auth 06",
		Password:    "StrongPass123!",
	}, nil, remoteAddr)
	require.Equal(t, http.StatusCreated, rec.Code)

	var regRes dto.AuthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &regRes))
	env.trackUser(regRes.User.ID)
	env.trackRedisKey("refresh:" + env.jwtMgr.HashRefreshToken(regRes.RefreshToken))

	for i := range 10 {
		rec = env.do(t, http.MethodPost, "/api/v1/auth/login", dto.LoginRequest{
			Login:    regRes.User.Username,
			Password: "WrongPassword!",
		}, nil, remoteAddr)
		assert.Equal(t, http.StatusUnauthorized, rec.Code, "attempt %d should return 401", i+1)
	}

	rec = env.do(t, http.MethodPost, "/api/v1/auth/login", dto.LoginRequest{
		Login:    regRes.User.Username,
		Password: "WrongPassword!",
	}, nil, remoteAddr)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))

	var errResp controller.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "RATE_LIMITED", errResp.Error.Code)

	ttl, err := env.redis.TTL(context.Background(), rlKey).Result()
	require.NoError(t, err)
	assert.Greater(t, ttl, time.Duration(0))
	assert.LessOrEqual(t, ttl, time.Minute)
}
