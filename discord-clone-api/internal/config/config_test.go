package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_DefaultsAndEnvOverrides(t *testing.T) {
	// Run inside an isolated empty temp dir so repo .env files do not interfere.
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	t.Setenv("PORT", "9090")
	t.Setenv("JWT_ACCESS_TTL", "20m")
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:5173, tauri://localhost")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:9090", cfg.Addr())
	assert.Equal(t, 20*time.Minute, cfg.AccessTokenTTL)
	assert.Equal(t, 7*24*time.Hour, cfg.RefreshTokenTTL)
	assert.Equal(t, 10, cfg.BcryptCost)
	assert.Equal(t, 10, cfg.LoginRateLimit)
	assert.Equal(t, time.Minute, cfg.LoginRateWindow)
	assert.Equal(t, 30, cfg.MessageRateLimit)
	assert.Equal(t, 10*time.Second, cfg.MessageRateWindow)
	assert.Equal(t, []string{"http://localhost:5173", "tauri://localhost"}, cfg.CORSAllowedOrigins)
}

func TestLoad_HostBindingByEnv(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	t.Setenv("HOST", "0.0.0.0")
	t.Setenv("PORT", "8080")
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "0.0.0.0:8080", cfg.Addr())

	t.Setenv("HOST", "")
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "production-secret-key-at-least-32-characters")
	prodCfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, ":8080", prodCfg.Addr())
}

func TestLoad_ReadsDotEnvWithoutOverridingOS(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	envPath := filepath.Join(tmpDir, ".env")
	err := os.WriteFile(envPath, []byte("# comment\nPORT=7070\nREDIS_ADDR=\"127.0.0.1:6380\"\n"), 0o600)
	require.NoError(t, err)

	// Pre-set REDIS_ADDR in OS env; ensure .env does not overwrite it, while PORT comes from .env.
	t.Setenv("REDIS_ADDR", "redis.internal:6379")
	// Clear PORT if present in parent environment and clean it up after test.
	if orig, existed := os.LookupEnv("PORT"); existed {
		require.NoError(t, os.Unsetenv("PORT"))
		t.Cleanup(func() { _ = os.Setenv("PORT", orig) })
	} else {
		t.Cleanup(func() { _ = os.Unsetenv("PORT") })
	}

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "7070", cfg.Port)
	assert.Equal(t, "redis.internal:6379", cfg.RedisAddr)
}

func TestLoad_ValidationErrors(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	t.Run("production rejects default secret", func(t *testing.T) {
		t.Setenv("APP_ENV", "production")
		_, err := Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "JWT_SECRET")
	})

	t.Run("invalid bcrypt cost", func(t *testing.T) {
		t.Setenv("BCRYPT_COST", "99")
		_, err := Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "BCRYPT_COST")
	})

	t.Run("invalid duration", func(t *testing.T) {
		t.Setenv("JWT_ACCESS_TTL", "not-a-duration")
		_, err := Load()
		require.Error(t, err)
	})
}
