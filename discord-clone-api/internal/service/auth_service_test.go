package service

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/truong/discord-clone-api/internal/domain"
	"github.com/truong/discord-clone-api/pkg/hasher"
	"github.com/truong/discord-clone-api/pkg/jwtutil"
	"golang.org/x/crypto/bcrypt"
)

type memoryUserRepo struct {
	mu    sync.RWMutex
	users map[uuid.UUID]domain.User
}

func newMemoryUserRepo() *memoryUserRepo {
	return &memoryUserRepo{
		users: make(map[uuid.UUID]domain.User),
	}
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
	copyU := u
	return &copyU, nil
}

func (r *memoryUserRepo) GetByLogin(_ context.Context, login string) (*domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, u := range r.users {
		if strings.EqualFold(u.Username, login) || strings.EqualFold(u.Email, login) {
			copyU := u
			return &copyU, nil
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
	copyU := u
	return &copyU, nil
}

type memoryTokenEntry struct {
	userID    uuid.UUID
	expiresAt time.Time
}

type memoryTokenStore struct {
	mu      sync.Mutex
	entries map[string]memoryTokenEntry
}

func newMemoryTokenStore() *memoryTokenStore {
	return &memoryTokenStore{
		entries: make(map[string]memoryTokenEntry),
	}
}

func (s *memoryTokenStore) SaveRefreshToken(_ context.Context, tokenHash string, userID uuid.UUID, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries[tokenHash] = memoryTokenEntry{
		userID:    userID,
		expiresAt: time.Now().Add(ttl),
	}
	return nil
}

func (s *memoryTokenStore) ConsumeRefreshToken(_ context.Context, tokenHash string) (uuid.UUID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.entries[tokenHash]
	if !ok {
		return uuid.Nil, domain.NewError(domain.ErrInvalidToken, "INVALID_REFRESH_TOKEN", "refresh token is invalid or expired")
	}
	delete(s.entries, tokenHash)
	if time.Now().After(entry.expiresAt) {
		return uuid.Nil, domain.NewError(domain.ErrInvalidToken, "INVALID_REFRESH_TOKEN", "refresh token is expired")
	}
	return entry.userID, nil
}

func (s *memoryTokenStore) DeleteRefreshToken(_ context.Context, tokenHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.entries, tokenHash)
	return nil
}

func newTestAuthService() (*AuthService, *memoryUserRepo, *memoryTokenStore, *jwtutil.Manager) {
	repo := newMemoryUserRepo()
	store := newMemoryTokenStore()
	h := hasher.NewBcryptHasher(bcrypt.MinCost)
	jwtMgr := jwtutil.NewManager("unit-test-secret-key-32-bytes-minimum!", 15*time.Minute)
	svc := NewAuthService(repo, store, h, jwtMgr, 7*24*time.Hour)
	return svc, repo, store, jwtMgr
}

func TestAuthService_Register_Success(t *testing.T) {
	svc, repo, store, jwtMgr := newTestAuthService()
	ctx := context.Background()

	res, err := svc.Register(ctx, RegisterInput{
		Username:    "truong_dev",
		Email:       "truong@example.com",
		DisplayName: "Truong",
		Password:    "Password123!",
	})
	require.NoError(t, err)
	assert.Equal(t, uuid.Version(7), res.User.ID.Version())
	assert.Equal(t, "truong_dev", res.User.Username)
	assert.NotEmpty(t, res.AccessToken)
	assert.NotEmpty(t, res.RefreshToken)

	parsedID, err := jwtMgr.ParseAccessToken(res.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, res.User.ID, parsedID)

	stored, err := repo.GetByID(ctx, res.User.ID)
	require.NoError(t, err)
	assert.NotEqual(t, "Password123!", stored.PasswordHash)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte("Password123!")))

	hash := jwtMgr.HashRefreshToken(res.RefreshToken)
	_, ok := store.entries[hash]
	assert.True(t, ok, "refresh token hash should be saved in TokenStore")
}

func TestAuthService_Register_ValidationErrors(t *testing.T) {
	svc, _, _, _ := newTestAuthService()
	ctx := context.Background()

	cases := []struct {
		name  string
		input RegisterInput
	}{
		{"short username", RegisterInput{"ab", "a@b.com", "Name", "Password123"}},
		{"invalid chars username", RegisterInput{"bad user!", "a@b.com", "Name", "Password123"}},
		{"invalid email", RegisterInput{"valid_user", "not-an-email", "Name", "Password123"}},
		{"empty displayName", RegisterInput{"valid_user", "a@b.com", "   ", "Password123"}},
		{"long displayName", RegisterInput{"valid_user", "a@b.com", strings.Repeat("a", 65), "Password123"}},
		{"short password", RegisterInput{"valid_user", "a@b.com", "Name", "1234567"}},
		{"password > 72 bytes", RegisterInput{"valid_user", "a@b.com", "Name", strings.Repeat("p", 73)}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Register(ctx, tc.input)
			require.Error(t, err)
			assert.ErrorIs(t, err, domain.ErrValidation)
		})
	}
}

func TestAuthService_Register_DuplicateCaseInsensitive(t *testing.T) {
	svc, _, _, _ := newTestAuthService()
	ctx := context.Background()

	_, err := svc.Register(ctx, RegisterInput{
		Username:    "Truong",
		Email:       "Truong@Example.com",
		DisplayName: "Truong",
		Password:    "Password123!",
	})
	require.NoError(t, err)

	_, err = svc.Register(ctx, RegisterInput{
		Username:    "truong",
		Email:       "other@example.com",
		DisplayName: "Other",
		Password:    "Password123!",
	})
	assert.ErrorIs(t, err, domain.ErrConflict)

	_, err = svc.Register(ctx, RegisterInput{
		Username:    "other_user",
		Email:       "truong@example.com",
		DisplayName: "Other",
		Password:    "Password123!",
	})
	assert.ErrorIs(t, err, domain.ErrConflict)
}

func TestAuthService_Login_SuccessAndFailures(t *testing.T) {
	svc, _, _, _ := newTestAuthService()
	ctx := context.Background()

	reg, err := svc.Register(ctx, RegisterInput{
		Username:    "Alice",
		Email:       "Alice@Example.com",
		DisplayName: "Alice",
		Password:    "SecretPass99",
	})
	require.NoError(t, err)

	// By username (case-insensitive)
	res1, err := svc.Login(ctx, LoginInput{Login: "alice", Password: "SecretPass99"})
	require.NoError(t, err)
	assert.Equal(t, reg.User.ID, res1.User.ID)

	// By email (case-insensitive)
	res2, err := svc.Login(ctx, LoginInput{Login: "alice@example.com", Password: "SecretPass99"})
	require.NoError(t, err)
	assert.Equal(t, reg.User.ID, res2.User.ID)

	// Wrong password
	_, err = svc.Login(ctx, LoginInput{Login: "alice", Password: "WrongPassword"})
	assert.ErrorIs(t, err, domain.ErrInvalidCredentials)

	// Unknown user
	_, err = svc.Login(ctx, LoginInput{Login: "nobody", Password: "SecretPass99"})
	assert.ErrorIs(t, err, domain.ErrInvalidCredentials)
}

func TestAuthService_Refresh_RotationAndReuseRejected(t *testing.T) {
	svc, _, _, _ := newTestAuthService()
	ctx := context.Background()

	reg, err := svc.Register(ctx, RegisterInput{
		Username:    "bob_user",
		Email:       "bob@example.com",
		DisplayName: "Bob",
		Password:    "Password123!",
	})
	require.NoError(t, err)

	rotated, err := svc.Refresh(ctx, reg.RefreshToken)
	require.NoError(t, err)
	assert.NotEqual(t, reg.RefreshToken, rotated.RefreshToken)
	assert.Equal(t, reg.User.ID, rotated.User.ID)

	// Reusing old refresh token must fail
	_, err = svc.Refresh(ctx, reg.RefreshToken)
	assert.ErrorIs(t, err, domain.ErrInvalidToken)

	// New refresh token must still work
	rotated2, err := svc.Refresh(ctx, rotated.RefreshToken)
	require.NoError(t, err)
	assert.NotEqual(t, rotated.RefreshToken, rotated2.RefreshToken)
}

func TestAuthService_Refresh_ConcurrentRotation(t *testing.T) {
	svc, _, _, _ := newTestAuthService()
	ctx := context.Background()

	reg, err := svc.Register(ctx, RegisterInput{
		Username:    "race_user",
		Email:       "race@example.com",
		DisplayName: "Race",
		Password:    "Password123!",
	})
	require.NoError(t, err)

	const workers = 8
	var wg sync.WaitGroup
	wg.Add(workers)
	results := make(chan error, workers)

	for range workers {
		go func() {
			defer wg.Done()
			_, err := svc.Refresh(ctx, reg.RefreshToken)
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	successCount := 0
	failCount := 0
	for err := range results {
		if err == nil {
			successCount++
		} else {
			assert.ErrorIs(t, err, domain.ErrInvalidToken)
			failCount++
		}
	}

	assert.Equal(t, 1, successCount)
	assert.Equal(t, workers-1, failCount)
}

func TestAuthService_Logout_RevokesRefreshToken(t *testing.T) {
	svc, _, _, _ := newTestAuthService()
	ctx := context.Background()

	reg, err := svc.Register(ctx, RegisterInput{
		Username:    "logout_user",
		Email:       "logout@example.com",
		DisplayName: "Logout",
		Password:    "Password123!",
	})
	require.NoError(t, err)

	require.NoError(t, svc.Logout(ctx, reg.RefreshToken))

	_, err = svc.Refresh(ctx, reg.RefreshToken)
	assert.ErrorIs(t, err, domain.ErrInvalidToken)
}

func TestAuthService_GetAndUpdateMe(t *testing.T) {
	svc, _, _, _ := newTestAuthService()
	ctx := context.Background()

	reg, err := svc.Register(ctx, RegisterInput{
		Username:    "profile_user",
		Email:       "profile@example.com",
		DisplayName: "Old Name",
		Password:    "Password123!",
	})
	require.NoError(t, err)

	me, err := svc.GetMe(ctx, reg.User.ID)
	require.NoError(t, err)
	assert.Equal(t, "Old Name", me.DisplayName)
	assert.Nil(t, me.AvatarURL)

	newName := "New Name"
	avatar := "https://cdn.example.com/avatar.png"
	updated, err := svc.UpdateMe(ctx, reg.User.ID, UpdateProfileInput{
		DisplayName:  &newName,
		AvatarURL:    &avatar,
		AvatarURLSet: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "New Name", updated.DisplayName)
	require.NotNil(t, updated.AvatarURL)
	assert.Equal(t, avatar, *updated.AvatarURL)

	// Omit avatarUrl -> preserves existing avatar
	newerName := "Newer Name"
	updated2, err := svc.UpdateMe(ctx, reg.User.ID, UpdateProfileInput{
		DisplayName:  &newerName,
		AvatarURLSet: false,
	})
	require.NoError(t, err)
	assert.Equal(t, "Newer Name", updated2.DisplayName)
	require.NotNil(t, updated2.AvatarURL)
	assert.Equal(t, avatar, *updated2.AvatarURL)

	// Explicit nil avatarUrl -> clears avatar
	updated3, err := svc.UpdateMe(ctx, reg.User.ID, UpdateProfileInput{
		AvatarURL:    nil,
		AvatarURLSet: true,
	})
	require.NoError(t, err)
	assert.Nil(t, updated3.AvatarURL)

	// Invalid avatarUrl rejected
	badAvatar := "ftp://example.com/bad.png"
	_, err = svc.UpdateMe(ctx, reg.User.ID, UpdateProfileInput{
		AvatarURL:    &badAvatar,
		AvatarURLSet: true,
	})
	assert.ErrorIs(t, err, domain.ErrValidation)

	// Unknown user
	_, err = svc.GetMe(ctx, uuid.Must(uuid.NewV7()))
	assert.ErrorIs(t, err, domain.ErrNotFound)
}
