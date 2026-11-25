package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const defaultDevJWTSecret = "dev-only-jwt-secret-key-change-in-production-32b"

// Config holds all runtime configuration for the API server.
type Config struct {
	AppEnv             string
	Host               string
	Port               string
	LogLevel           string
	DatabaseURL        string
	RedisAddr          string
	RedisPassword      string
	RedisDB            int
	JWTSecret          string
	AccessTokenTTL     time.Duration
	RefreshTokenTTL    time.Duration
	BcryptCost         int
	CORSAllowedOrigins []string
	LoginRateLimit     int
	LoginRateWindow    time.Duration
	MessageRateLimit    int
	MessageRateWindow   time.Duration
	WSTicketTTL         time.Duration
	WSHeartbeatInterval time.Duration
	ShutdownTimeout     time.Duration
}

// Addr returns the HTTP listen address formatted as "host:port" (or ":port" when Host is empty).
func (c *Config) Addr() string {
	port := strings.TrimPrefix(strings.TrimSpace(c.Port), ":")
	host := strings.TrimSpace(c.Host)
	if host == "" {
		return ":" + port
	}
	return host + ":" + port
}

// Load reads optional .env files (without overriding existing OS env vars)
// and parses environment configuration with development defaults.
func Load() (*Config, error) {
	if err := loadEnvFiles(".env", "deployments/.env"); err != nil {
		return nil, err
	}

	redisDB, err := getEnvInt("REDIS_DB", 0)
	if err != nil {
		return nil, err
	}
	accessTTL, err := getEnvDuration("JWT_ACCESS_TTL", 15*time.Minute)
	if err != nil {
		return nil, err
	}
	refreshTTL, err := getEnvDuration("JWT_REFRESH_TTL", 7*24*time.Hour)
	if err != nil {
		return nil, err
	}
	bcryptCost, err := getEnvInt("BCRYPT_COST", 10)
	if err != nil {
		return nil, err
	}
	loginRateLimit, err := getEnvInt("LOGIN_RATE_LIMIT", 10)
	if err != nil {
		return nil, err
	}
	loginRateWindow, err := getEnvDuration("LOGIN_RATE_WINDOW", time.Minute)
	if err != nil {
		return nil, err
	}
	messageRateLimit, err := getEnvInt("MESSAGE_RATE_LIMIT", 30)
	if err != nil {
		return nil, err
	}
	messageRateWindow, err := getEnvDuration("MESSAGE_RATE_WINDOW", 10*time.Second)
	if err != nil {
		return nil, err
	}
	wsTicketTTL, err := getEnvDuration("WS_TICKET_TTL", 30*time.Second)
	if err != nil {
		return nil, err
	}
	wsHeartbeatInterval, err := getEnvDuration("WS_HEARTBEAT_INTERVAL", 25*time.Second)
	if err != nil {
		return nil, err
	}
	shutdownTimeout, err := getEnvDuration("SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		return nil, err
	}

	appEnv := getEnvString("APP_ENV", "development")
	defaultHost := "127.0.0.1"
	if strings.EqualFold(appEnv, "production") {
		defaultHost = ""
	}

	cfg := &Config{
		AppEnv:              appEnv,
		Host:                getEnvString("HOST", defaultHost),
		Port:                getEnvString("PORT", "8080"),
		LogLevel:            getEnvString("LOG_LEVEL", "info"),
		DatabaseURL:         getEnvString("DATABASE_URL", "postgres://discord:discord_secret_dev@localhost:5432/discord_clone?sslmode=disable"),
		RedisAddr:           getEnvString("REDIS_ADDR", "localhost:6379"),
		RedisPassword:       getEnvString("REDIS_PASSWORD", ""),
		RedisDB:             redisDB,
		JWTSecret:           getEnvString("JWT_SECRET", defaultDevJWTSecret),
		AccessTokenTTL:      accessTTL,
		RefreshTokenTTL:     refreshTTL,
		BcryptCost:          bcryptCost,
		CORSAllowedOrigins:  getEnvCSV("CORS_ALLOWED_ORIGINS", []string{"http://localhost:5173", "http://127.0.0.1:5173", "tauri://localhost", "http://tauri.localhost", "https://tauri.localhost"}),
		LoginRateLimit:      loginRateLimit,
		LoginRateWindow:     loginRateWindow,
		MessageRateLimit:    messageRateLimit,
		MessageRateWindow:   messageRateWindow,
		WSTicketTTL:         wsTicketTTL,
		WSHeartbeatInterval: wsHeartbeatInterval,
		ShutdownTimeout:     shutdownTimeout,
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if strings.TrimSpace(c.Port) == "" {
		return errors.New("PORT cannot be empty")
	}
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return errors.New("DATABASE_URL cannot be empty")
	}
	if strings.TrimSpace(c.RedisAddr) == "" {
		return errors.New("REDIS_ADDR cannot be empty")
	}
	if c.RedisDB < 0 {
		return errors.New("REDIS_DB must be non-negative")
	}
	if strings.TrimSpace(c.JWTSecret) == "" {
		return errors.New("JWT_SECRET cannot be empty")
	}
	if strings.EqualFold(c.AppEnv, "production") {
		if c.JWTSecret == defaultDevJWTSecret || len(c.JWTSecret) < 32 {
			return errors.New("JWT_SECRET must be a custom secret of at least 32 characters in production")
		}
	}
	if c.AccessTokenTTL <= 0 {
		return errors.New("JWT_ACCESS_TTL must be positive")
	}
	if c.RefreshTokenTTL <= 0 {
		return errors.New("JWT_REFRESH_TTL must be positive")
	}
	if c.BcryptCost < bcrypt.MinCost || c.BcryptCost > bcrypt.MaxCost {
		return fmt.Errorf("BCRYPT_COST must be between %d and %d", bcrypt.MinCost, bcrypt.MaxCost)
	}
	if c.LoginRateLimit <= 0 {
		return errors.New("LOGIN_RATE_LIMIT must be positive")
	}
	if c.LoginRateWindow <= 0 {
		return errors.New("LOGIN_RATE_WINDOW must be positive")
	}
	if c.MessageRateLimit <= 0 {
		return errors.New("MESSAGE_RATE_LIMIT must be positive")
	}
	if c.MessageRateWindow <= 0 {
		return errors.New("MESSAGE_RATE_WINDOW must be positive")
	}
	if c.WSTicketTTL <= 0 {
		return errors.New("WS_TICKET_TTL must be positive")
	}
	if c.WSHeartbeatInterval <= 0 {
		return errors.New("WS_HEARTBEAT_INTERVAL must be positive")
	}
	if c.ShutdownTimeout <= 0 {
		return errors.New("SHUTDOWN_TIMEOUT must be positive")
	}
	return nil
}

func loadEnvFiles(paths ...string) error {
	for _, p := range paths {
		if err := loadEnvFile(p); err != nil {
			return err
		}
	}
	return nil
}

func loadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("open env file %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if key == "" {
			continue
		}
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, val); err != nil {
				return fmt.Errorf("set env %s: %w", key, err)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan env file %s: %w", path, err)
	}
	return nil
}

func getEnvString(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) (int, error) {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return parsed, nil
}

func getEnvDuration(key string, fallback time.Duration) (time.Duration, error) {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(strings.TrimSpace(v))
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return parsed, nil
}

func getEnvCSV(key string, fallback []string) []string {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return fallback
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}
