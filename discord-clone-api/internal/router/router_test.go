package router

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
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
	"github.com/truong/discord-clone-api/internal/gateway"
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

type memoryGuildRepo struct {
	mu       sync.RWMutex
	guilds   map[uuid.UUID]domain.Guild
	channels map[uuid.UUID]domain.Channel
	members  map[uuid.UUID]map[uuid.UUID]domain.GuildMember
	users    *memoryUserRepo
}

func newMemoryGuildRepo(users *memoryUserRepo) *memoryGuildRepo {
	return &memoryGuildRepo{
		guilds:   make(map[uuid.UUID]domain.Guild),
		channels: make(map[uuid.UUID]domain.Channel),
		members:  make(map[uuid.UUID]map[uuid.UUID]domain.GuildMember),
		users:    users,
	}
}

func (r *memoryGuildRepo) CreateWithDefaultChannel(_ context.Context, guild *domain.Guild, ch *domain.Channel, owner *domain.GuildMember) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	guild.CreatedAt = now
	ch.CreatedAt = now
	owner.JoinedAt = now
	r.guilds[guild.ID] = *guild
	r.channels[ch.ID] = *ch
	r.members[guild.ID] = map[uuid.UUID]domain.GuildMember{owner.UserID: *owner}
	return nil
}

func (r *memoryGuildRepo) ListByUser(_ context.Context, userID uuid.UUID) ([]domain.Guild, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Guild, 0)
	for gid, mems := range r.members {
		if _, ok := mems[userID]; ok {
			out = append(out, r.guilds[gid])
		}
	}
	return out, nil
}

func (r *memoryGuildRepo) GetByID(_ context.Context, guildID uuid.UUID) (*domain.Guild, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	g, ok := r.guilds[guildID]
	if !ok {
		return nil, domain.NewError(domain.ErrNotFound, "GUILD_NOT_FOUND", "guild not found")
	}
	cp := g
	return &cp, nil
}

func (r *memoryGuildRepo) Update(_ context.Context, guildID uuid.UUID, params domain.UpdateGuildParams) (*domain.Guild, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g, ok := r.guilds[guildID]
	if !ok {
		return nil, domain.NewError(domain.ErrNotFound, "GUILD_NOT_FOUND", "guild not found")
	}
	if params.Name != nil {
		g.Name = *params.Name
	}
	if params.IconURLSet {
		g.IconURL = params.IconURL
	}
	r.guilds[guildID] = g
	cp := g
	return &cp, nil
}

func (r *memoryGuildRepo) Delete(_ context.Context, guildID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.guilds[guildID]; !ok {
		return domain.NewError(domain.ErrNotFound, "GUILD_NOT_FOUND", "guild not found")
	}
	delete(r.guilds, guildID)
	delete(r.members, guildID)
	return nil
}

func (r *memoryGuildRepo) GetMember(_ context.Context, guildID, userID uuid.UUID) (*domain.GuildMember, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.members[guildID][userID]
	if !ok {
		return nil, domain.NewError(domain.ErrNotFound, "MEMBER_NOT_FOUND", "member not found")
	}
	cp := m
	return &cp, nil
}

func (r *memoryGuildRepo) ListMembers(ctx context.Context, guildID uuid.UUID) ([]domain.GuildMember, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.GuildMember, 0, len(r.members[guildID]))
	for _, m := range r.members[guildID] {
		if u, err := r.users.GetByID(ctx, m.UserID); err == nil {
			m.User = *u
		}
		out = append(out, m)
	}
	return out, nil
}

func (r *memoryGuildRepo) RemoveMember(_ context.Context, guildID, userID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.members[guildID][userID]; !ok {
		return domain.NewError(domain.ErrNotFound, "MEMBER_NOT_FOUND", "member not found")
	}
	delete(r.members[guildID], userID)
	return nil
}

type memoryInviteRepo struct {
	mu      sync.Mutex
	guilds  *memoryGuildRepo
	invites map[string]domain.Invite
}

func newMemoryInviteRepo(guilds *memoryGuildRepo) *memoryInviteRepo {
	return &memoryInviteRepo{guilds: guilds, invites: make(map[string]domain.Invite)}
}

func (r *memoryInviteRepo) Create(_ context.Context, inv *domain.Invite) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.invites[inv.Code]; ok {
		return domain.NewError(domain.ErrConflict, "INVITE_CODE_CONFLICT", "invite code already exists")
	}
	inv.CreatedAt = time.Now().UTC()
	r.invites[inv.Code] = *inv
	return nil
}

func (r *memoryInviteRepo) Accept(ctx context.Context, code string, userID uuid.UUID, now time.Time) (*domain.Guild, *domain.GuildMember, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	inv, ok := r.invites[code]
	if !ok {
		return nil, nil, domain.NewError(domain.ErrNotFound, "INVITE_NOT_FOUND", "invite code not found")
	}
	if inv.ExpiresAt != nil && !inv.ExpiresAt.After(now) {
		return nil, nil, domain.NewError(domain.ErrValidation, "INVITE_EXPIRED", "invite code has expired")
	}
	if inv.MaxUses != nil && inv.Uses >= *inv.MaxUses {
		return nil, nil, domain.NewError(domain.ErrValidation, "INVITE_EXHAUSTED", "invite code has reached its maximum uses")
	}

	r.guilds.mu.Lock()
	defer r.guilds.mu.Unlock()
	if _, already := r.guilds.members[inv.GuildID][userID]; already {
		return nil, nil, domain.NewError(domain.ErrConflict, "ALREADY_MEMBER", "you are already a member of this guild")
	}
	inv.Uses++
	r.invites[code] = inv
	u, err := r.guilds.users.GetByID(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	member := domain.GuildMember{GuildID: inv.GuildID, UserID: userID, Role: domain.RoleMember, JoinedAt: now, User: *u}
	r.guilds.members[inv.GuildID][userID] = member
	g := r.guilds.guilds[inv.GuildID]
	return &g, &member, nil
}

type memoryChannelRepo struct {
	guilds *memoryGuildRepo
}

func newMemoryChannelRepo(guilds *memoryGuildRepo) *memoryChannelRepo {
	return &memoryChannelRepo{guilds: guilds}
}

func (r *memoryChannelRepo) Create(_ context.Context, ch *domain.Channel) error {
	r.guilds.mu.Lock()
	defer r.guilds.mu.Unlock()
	maxPos := -1
	for _, existing := range r.guilds.channels {
		if existing.GuildID == ch.GuildID {
			if existing.Name == ch.Name {
				return domain.NewError(domain.ErrConflict, "CHANNEL_NAME_TAKEN", "channel name already exists in this guild")
			}
			if existing.Position > maxPos {
				maxPos = existing.Position
			}
		}
	}
	ch.Position = maxPos + 1
	ch.CreatedAt = time.Now().UTC()
	r.guilds.channels[ch.ID] = *ch
	return nil
}

func (r *memoryChannelRepo) ListByGuild(_ context.Context, guildID uuid.UUID) ([]domain.Channel, error) {
	r.guilds.mu.RLock()
	defer r.guilds.mu.RUnlock()
	out := make([]domain.Channel, 0)
	for _, ch := range r.guilds.channels {
		if ch.GuildID == guildID {
			out = append(out, ch)
		}
	}
	return out, nil
}

func (r *memoryChannelRepo) GetByID(_ context.Context, channelID uuid.UUID) (*domain.Channel, error) {
	r.guilds.mu.RLock()
	defer r.guilds.mu.RUnlock()
	ch, ok := r.guilds.channels[channelID]
	if !ok {
		return nil, domain.NewError(domain.ErrNotFound, "CHANNEL_NOT_FOUND", "channel not found")
	}
	cp := ch
	return &cp, nil
}

func (r *memoryChannelRepo) Update(_ context.Context, channelID uuid.UUID, params domain.UpdateChannelParams) (*domain.Channel, error) {
	r.guilds.mu.Lock()
	defer r.guilds.mu.Unlock()
	ch, ok := r.guilds.channels[channelID]
	if !ok {
		return nil, domain.NewError(domain.ErrNotFound, "CHANNEL_NOT_FOUND", "channel not found")
	}
	if params.Name != nil {
		for _, other := range r.guilds.channels {
			if other.GuildID == ch.GuildID && other.ID != ch.ID && other.Name == *params.Name {
				return nil, domain.NewError(domain.ErrConflict, "CHANNEL_NAME_TAKEN", "channel name already exists in this guild")
			}
		}
		ch.Name = *params.Name
	}
	if params.TopicSet {
		ch.Topic = params.Topic
	}
	r.guilds.channels[channelID] = ch
	cp := ch
	return &cp, nil
}

func (r *memoryChannelRepo) Delete(_ context.Context, channelID uuid.UUID) error {
	r.guilds.mu.Lock()
	defer r.guilds.mu.Unlock()
	if _, ok := r.guilds.channels[channelID]; !ok {
		return domain.NewError(domain.ErrNotFound, "CHANNEL_NOT_FOUND", "channel not found")
	}
	delete(r.guilds.channels, channelID)
	return nil
}

type memoryMessageRepo struct {
	mu       sync.RWMutex
	messages map[uuid.UUID]domain.Message
	users    *memoryUserRepo
}

func newMemoryMessageRepo(users *memoryUserRepo) *memoryMessageRepo {
	return &memoryMessageRepo{
		messages: make(map[uuid.UUID]domain.Message),
		users:    users,
	}
}

func (r *memoryMessageRepo) Create(ctx context.Context, msg *domain.Message) (*domain.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	stored := *msg
	stored.CreatedAt = time.Now().UTC()
	if u, err := r.users.GetByID(ctx, msg.AuthorID); err == nil {
		stored.Author = domain.MessageAuthor{
			ID:          u.ID,
			Username:    u.Username,
			DisplayName: u.DisplayName,
			AvatarURL:   u.AvatarURL,
		}
	}
	r.messages[stored.ID] = stored
	cp := stored
	return &cp, nil
}

func (r *memoryMessageRepo) ListByChannel(_ context.Context, channelID uuid.UUID, before *uuid.UUID, limit int) ([]domain.Message, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Message, 0)
	for _, m := range r.messages {
		if m.ChannelID != channelID || m.DeletedAt != nil {
			continue
		}
		if before != nil && bytes.Compare(m.ID[:], (*before)[:]) >= 0 {
			continue
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		return bytes.Compare(out[i].ID[:], out[j].ID[:]) > 0
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *memoryMessageRepo) GetByID(_ context.Context, messageID uuid.UUID) (*domain.Message, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.messages[messageID]
	if !ok || m.DeletedAt != nil {
		return nil, domain.NewError(domain.ErrNotFound, "MESSAGE_NOT_FOUND", "message not found")
	}
	cp := m
	return &cp, nil
}

func (r *memoryMessageRepo) UpdateContent(_ context.Context, messageID uuid.UUID, content string) (*domain.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.messages[messageID]
	if !ok || m.DeletedAt != nil {
		return nil, domain.NewError(domain.ErrNotFound, "MESSAGE_NOT_FOUND", "message not found")
	}
	now := time.Now().UTC()
	m.Content = content
	m.EditedAt = &now
	r.messages[messageID] = m
	cp := m
	return &cp, nil
}

func (r *memoryMessageRepo) SoftDelete(_ context.Context, messageID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.messages[messageID]
	if !ok || m.DeletedAt != nil {
		return domain.NewError(domain.ErrNotFound, "MESSAGE_NOT_FOUND", "message not found")
	}
	now := time.Now().UTC()
	m.DeletedAt = &now
	r.messages[messageID] = m
	return nil
}

func newTestRouter(pgErr, redisErr error) (*gin.Engine, *jwtutil.Manager) {
	return newTestRouterWithCleanup(nil, pgErr, redisErr)
}

func newTestRouterWithCleanup(t *testing.T, pgErr, redisErr error) (*gin.Engine, *jwtutil.Manager) {
	cfg := &config.Config{
		AppEnv:              "test",
		Port:                "8080",
		LogLevel:            "error",
		JWTSecret:           "router-test-secret-key-at-least-32-bytes!",
		AccessTokenTTL:      15 * time.Minute,
		RefreshTokenTTL:     7 * 24 * time.Hour,
		BcryptCost:          bcrypt.MinCost,
		CORSAllowedOrigins:  []string{"http://localhost:5173", "tauri://localhost", "http://tauri.localhost", "https://tauri.localhost"},
		LoginRateLimit:      10,
		LoginRateWindow:     time.Minute,
		MessageRateLimit:    30,
		MessageRateWindow:   10 * time.Second,
		WSTicketTTL:         30 * time.Second,
		WSHeartbeatInterval: 25 * time.Second,
	}

	log := logger.NewWithWriter(io.Discard, "error")
	jwtMgr := jwtutil.NewManager(cfg.JWTSecret, cfg.AccessTokenTTL)
	userRepo := newMemoryUserRepo()
	guildRepo := newMemoryGuildRepo(userRepo)
	channelRepo := newMemoryChannelRepo(guildRepo)
	messageRepo := newMemoryMessageRepo(userRepo)
	inviteRepo := newMemoryInviteRepo(guildRepo)
	rateLimiter := newMemoryRateLimiter()
	ticketStore := newMemoryTicketStore()

	authSvc := service.NewAuthService(
		userRepo,
		newMemoryTokenStore(),
		hasher.NewBcryptHasher(cfg.BcryptCost),
		jwtMgr,
		cfg.RefreshTokenTTL,
	)
	guildSvc := service.NewGuildService(guildRepo, nil, domain.NoopGuildSubscriptionManager{}, domain.NoopGuildEventPublisher{})
	inviteSvc := service.NewInviteService(guildRepo, inviteRepo, domain.NoopGuildSubscriptionManager{}, domain.NoopGuildEventPublisher{})
	channelSvc := service.NewChannelService(guildRepo, channelRepo, domain.NoopChannelEventPublisher{})
	messageSvc := service.NewMessageService(
		guildRepo,
		channelRepo,
		messageRepo,
		rateLimiter,
		domain.NoopMessageEventPublisher{},
		cfg.MessageRateLimit,
		cfg.MessageRateWindow,
	)
	ticketSvc := service.NewTicketService(ticketStore, cfg.WSTicketTTL)

	var wsHandler gin.HandlerFunc
	if t != nil {
		ctx, cancel := context.WithCancel(context.Background())
		hub := gateway.NewHub(cfg.WSHeartbeatInterval, log)
		hubDone := make(chan struct{})
		go func() {
			defer close(hubDone)
			hub.Run(ctx)
		}()
		t.Cleanup(func() {
			cancel()
			<-hubDone
		})
		wsHandler = gateway.NewHandler(hub, ticketSvc, guildRepo, cfg.CORSAllowedOrigins, log).ServeGin
	}

	r := New(Dependencies{
		Config:             cfg,
		Logger:             log,
		TokenManager:       jwtMgr,
		RateLimiter:        rateLimiter,
		HealthController:   controller.NewHealthController(&stubHealthChecker{err: pgErr}, &stubHealthChecker{err: redisErr}, log),
		AuthController:     controller.NewAuthController(authSvc),
		UserController:     controller.NewUserController(authSvc, middleware.GetUserID),
		GuildController:    controller.NewGuildController(guildSvc, middleware.GetUserID),
		InviteController:   controller.NewInviteController(inviteSvc, middleware.GetUserID),
		ChannelController:  controller.NewChannelController(channelSvc, middleware.GetUserID),
		MessageController:  controller.NewMessageController(messageSvc, middleware.GetUserID),
		WSTicketController: controller.NewWSTicketController(ticketSvc, middleware.GetUserID),
		WSHandler:          wsHandler,
	})

	return r, jwtMgr
}

type memoryTicketStore struct {
	mu      sync.Mutex
	entries map[string]uuid.UUID
}

func newMemoryTicketStore() *memoryTicketStore {
	return &memoryTicketStore{entries: make(map[string]uuid.UUID)}
}

func (s *memoryTicketStore) SaveTicket(_ context.Context, ticket string, userID uuid.UUID, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[ticket] = userID
	return nil
}

func (s *memoryTicketStore) ConsumeTicket(_ context.Context, ticket string) (uuid.UUID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	uid, ok := s.entries[ticket]
	if !ok {
		return uuid.Nil, domain.NewError(domain.ErrInvalidToken, "INVALID_TICKET", "invalid ticket")
	}
	delete(s.entries, ticket)
	return uid, nil
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

func TestRouter_GuildAndInviteRoutes(t *testing.T) {
	r, _ := newTestRouter(nil, nil)

	// Register owner and second user
	rec := doJSONRequest(t, r, http.MethodPost, "/api/v1/auth/register", dto.RegisterRequest{
		Username:    "g_owner",
		Email:       "g_owner@example.com",
		DisplayName: "Guild Owner",
		Password:    "Password123!",
	}, nil)
	require.Equal(t, http.StatusCreated, rec.Code)
	var ownerAuth dto.AuthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ownerAuth))

	rec = doJSONRequest(t, r, http.MethodPost, "/api/v1/auth/register", dto.RegisterRequest{
		Username:    "g_member",
		Email:       "g_member@example.com",
		DisplayName: "Guild Member",
		Password:    "Password123!",
	}, nil)
	require.Equal(t, http.StatusCreated, rec.Code)
	var memberAuth dto.AuthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &memberAuth))

	ownerHdr := map[string]string{"Authorization": "Bearer " + ownerAuth.AccessToken}
	memberHdr := map[string]string{"Authorization": "Bearer " + memberAuth.AccessToken}

	// Unauthenticated / invalid UUID checks
	assert.Equal(t, http.StatusUnauthorized, doJSONRequest(t, r, http.MethodGet, "/api/v1/guilds", nil, nil).Code)
	assert.Equal(t, http.StatusBadRequest, doJSONRequest(t, r, http.MethodGet, "/api/v1/guilds/not-a-uuid", nil, ownerHdr).Code)

	// Create guild
	rec = doJSONRequest(t, r, http.MethodPost, "/api/v1/guilds", dto.CreateGuildRequest{Name: "Router Guild"}, ownerHdr)
	require.Equal(t, http.StatusCreated, rec.Code)
	var guild dto.GuildResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &guild))

	// Create invite (empty body) & accept
	rec = doJSONRequest(t, r, http.MethodPost, "/api/v1/guilds/"+guild.ID.String()+"/invites", nil, ownerHdr)
	require.Equal(t, http.StatusCreated, rec.Code)
	var inv dto.InviteResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &inv))

	rec = doJSONRequest(t, r, http.MethodPost, "/api/v1/invites/"+inv.Code+"/accept", nil, memberHdr)
	require.Equal(t, http.StatusOK, rec.Code)

	// Members list & leave rules
	rec = doJSONRequest(t, r, http.MethodGet, "/api/v1/guilds/"+guild.ID.String()+"/members", nil, memberHdr)
	require.Equal(t, http.StatusOK, rec.Code)
	var members []dto.GuildMemberResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &members))
	assert.Len(t, members, 2)

	assert.Equal(t, http.StatusForbidden, doJSONRequest(t, r, http.MethodDelete, "/api/v1/guilds/"+guild.ID.String()+"/members/@me", nil, ownerHdr).Code)
	assert.Equal(t, http.StatusNoContent, doJSONRequest(t, r, http.MethodDelete, "/api/v1/guilds/"+guild.ID.String()+"/members/@me", nil, memberHdr).Code)

	// Owner delete guild
	assert.Equal(t, http.StatusNoContent, doJSONRequest(t, r, http.MethodDelete, "/api/v1/guilds/"+guild.ID.String(), nil, ownerHdr).Code)
}

func TestRouter_ChannelRoutes(t *testing.T) {
	r, _ := newTestRouter(nil, nil)

	rec := doJSONRequest(t, r, http.MethodPost, "/api/v1/auth/register", dto.RegisterRequest{
		Username:    "ch_owner",
		Email:       "ch_owner@example.com",
		DisplayName: "Ch Owner",
		Password:    "Password123!",
	}, nil)
	require.Equal(t, http.StatusCreated, rec.Code)
	var ownerAuth dto.AuthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ownerAuth))

	rec = doJSONRequest(t, r, http.MethodPost, "/api/v1/auth/register", dto.RegisterRequest{
		Username:    "ch_member",
		Email:       "ch_member@example.com",
		DisplayName: "Ch Member",
		Password:    "Password123!",
	}, nil)
	require.Equal(t, http.StatusCreated, rec.Code)
	var memberAuth dto.AuthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &memberAuth))

	ownerHdr := map[string]string{"Authorization": "Bearer " + ownerAuth.AccessToken}
	memberHdr := map[string]string{"Authorization": "Bearer " + memberAuth.AccessToken}

	rec = doJSONRequest(t, r, http.MethodPost, "/api/v1/guilds", dto.CreateGuildRequest{Name: "Channel Router Guild"}, ownerHdr)
	require.Equal(t, http.StatusCreated, rec.Code)
	var guild dto.GuildResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &guild))

	rec = doJSONRequest(t, r, http.MethodPost, "/api/v1/guilds/"+guild.ID.String()+"/invites", nil, ownerHdr)
	require.Equal(t, http.StatusCreated, rec.Code)
	var inv dto.InviteResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &inv))

	require.Equal(t, http.StatusOK, doJSONRequest(t, r, http.MethodPost, "/api/v1/invites/"+inv.Code+"/accept", nil, memberHdr).Code)

	// Invalid UUID params -> 400
	assert.Equal(t, http.StatusBadRequest, doJSONRequest(t, r, http.MethodGet, "/api/v1/guilds/not-a-uuid/channels", nil, ownerHdr).Code)
	assert.Equal(t, http.StatusBadRequest, doJSONRequest(t, r, http.MethodPatch, "/api/v1/channels/not-a-uuid", map[string]any{"name": "x"}, ownerHdr).Code)

	// List includes default #general
	rec = doJSONRequest(t, r, http.MethodGet, "/api/v1/guilds/"+guild.ID.String()+"/channels", nil, memberHdr)
	require.Equal(t, http.StatusOK, rec.Code)
	var list []dto.ChannelResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Len(t, list, 1)
	assert.Equal(t, "general", list[0].Name)

	// Member create forbidden -> 403
	assert.Equal(t, http.StatusForbidden, doJSONRequest(t, r, http.MethodPost, "/api/v1/guilds/"+guild.ID.String()+"/channels", dto.CreateChannelRequest{Name: "nope"}, memberHdr).Code)

	// Owner creates channel -> 201
	topic := "API discussion"
	rec = doJSONRequest(t, r, http.MethodPost, "/api/v1/guilds/"+guild.ID.String()+"/channels", dto.CreateChannelRequest{Name: "backend", Topic: &topic}, ownerHdr)
	require.Equal(t, http.StatusCreated, rec.Code)
	var ch dto.ChannelResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ch))
	assert.Equal(t, "backend", ch.Name)
	require.NotNil(t, ch.Topic)
	assert.Equal(t, topic, *ch.Topic)

	// Duplicate channel name -> 409
	assert.Equal(t, http.StatusConflict, doJSONRequest(t, r, http.MethodPost, "/api/v1/guilds/"+guild.ID.String()+"/channels", dto.CreateChannelRequest{Name: "backend"}, ownerHdr).Code)

	// Owner updates and deletes channel -> 200 & 204
	rec = doJSONRequest(t, r, http.MethodPatch, "/api/v1/channels/"+ch.ID.String(), map[string]any{"name": "api-core", "topic": nil}, ownerHdr)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ch))
	assert.Equal(t, "api-core", ch.Name)
	assert.Nil(t, ch.Topic)

	assert.Equal(t, http.StatusNoContent, doJSONRequest(t, r, http.MethodDelete, "/api/v1/channels/"+ch.ID.String(), nil, ownerHdr).Code)
}

func TestRouter_MessageRoutes(t *testing.T) {
	r, _ := newTestRouter(nil, nil)

	rec := doJSONRequest(t, r, http.MethodPost, "/api/v1/auth/register", dto.RegisterRequest{
		Username:    "m_owner",
		Email:       "m_owner@example.com",
		DisplayName: "Msg Owner",
		Password:    "Password123!",
	}, nil)
	require.Equal(t, http.StatusCreated, rec.Code)
	var ownerAuth dto.AuthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ownerAuth))
	ownerHdr := map[string]string{"Authorization": "Bearer " + ownerAuth.AccessToken}

	rec = doJSONRequest(t, r, http.MethodPost, "/api/v1/guilds", dto.CreateGuildRequest{Name: "Msg Router Guild"}, ownerHdr)
	require.Equal(t, http.StatusCreated, rec.Code)
	var guild dto.GuildResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &guild))

	rec = doJSONRequest(t, r, http.MethodGet, "/api/v1/guilds/"+guild.ID.String()+"/channels", nil, ownerHdr)
	require.Equal(t, http.StatusOK, rec.Code)
	var channels []dto.ChannelResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &channels))
	channelID := channels[0].ID

	// Invalid cursor / limit / UUID checks -> 400
	assert.Equal(t, http.StatusBadRequest, doJSONRequest(t, r, http.MethodGet, "/api/v1/channels/bad-uuid/messages", nil, ownerHdr).Code)
	assert.Equal(t, http.StatusBadRequest, doJSONRequest(t, r, http.MethodGet, "/api/v1/channels/"+channelID.String()+"/messages?before=not-uuid", nil, ownerHdr).Code)
	assert.Equal(t, http.StatusBadRequest, doJSONRequest(t, r, http.MethodGet, "/api/v1/channels/"+channelID.String()+"/messages?limit=101", nil, ownerHdr).Code)

	// Send -> 201, Update -> 200, List -> 200, Delete -> 204
	nonce := "n-1"
	rec = doJSONRequest(t, r, http.MethodPost, "/api/v1/channels/"+channelID.String()+"/messages", dto.SendMessageRequest{
		Content: "Hello router",
		Nonce:   &nonce,
	}, ownerHdr)
	require.Equal(t, http.StatusCreated, rec.Code)
	var msg dto.MessageResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &msg))
	assert.Equal(t, "Hello router", msg.Content)
	require.NotNil(t, msg.Nonce)
	assert.Equal(t, "n-1", *msg.Nonce)

	rec = doJSONRequest(t, r, http.MethodPatch, "/api/v1/channels/"+channelID.String()+"/messages/"+msg.ID.String(), dto.UpdateMessageRequest{
		Content: "Updated router",
	}, ownerHdr)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &msg))
	assert.Equal(t, "Updated router", msg.Content)
	assert.NotNil(t, msg.EditedAt)

	assert.Equal(t, http.StatusNoContent, doJSONRequest(t, r, http.MethodDelete, "/api/v1/channels/"+channelID.String()+"/messages/"+msg.ID.String(), nil, ownerHdr).Code)

	// 29 more messages succeed, 31st hits 429 RATE_LIMITED with Retry-After
	for range 29 {
		require.Equal(t, http.StatusCreated, doJSONRequest(t, r, http.MethodPost, "/api/v1/channels/"+channelID.String()+"/messages", dto.SendMessageRequest{Content: "ok"}, ownerHdr).Code)
	}
	rec = doJSONRequest(t, r, http.MethodPost, "/api/v1/channels/"+channelID.String()+"/messages", dto.SendMessageRequest{Content: "too many"}, ownerHdr)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
}

func TestRouter_WebSocketTicketAndUpgrade(t *testing.T) {
	r, _ := newTestRouterWithCleanup(t, nil, nil)

	// Unauthenticated POST /api/v1/ws/ticket -> 401
	assert.Equal(t, http.StatusUnauthorized, doJSONRequest(t, r, http.MethodPost, "/api/v1/ws/ticket", nil, nil).Code)

	// Register user & issue ticket
	rec := doJSONRequest(t, r, http.MethodPost, "/api/v1/auth/register", dto.RegisterRequest{
		Username:    "ws_router",
		Email:       "ws_router@example.com",
		DisplayName: "WS Router",
		Password:    "Password123!",
	}, nil)
	require.Equal(t, http.StatusCreated, rec.Code)
	var authRes dto.AuthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &authRes))

	hdr := map[string]string{"Authorization": "Bearer " + authRes.AccessToken}
	rec = doJSONRequest(t, r, http.MethodPost, "/api/v1/ws/ticket", nil, hdr)
	require.Equal(t, http.StatusOK, rec.Code)
	var ticketResp dto.WSTicketResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ticketResp))
	assert.NotEmpty(t, ticketResp.Ticket)

	// Invalid ticket at GET /ws -> 401
	rec = doJSONRequest(t, r, http.MethodGet, "/ws?ticket=does-not-exist", nil, nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
