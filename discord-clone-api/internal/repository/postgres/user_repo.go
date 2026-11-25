package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/truong/discord-clone-api/internal/domain"
)

// UserRepository implements domain.UserRepository using PostgreSQL via pgxpool.
type UserRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository constructs a UserRepository backed by pool.
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// Create inserts a new user record and populates CreatedAt from the database.
func (r *UserRepository) Create(ctx context.Context, user *domain.User) error {
	const query = `
		INSERT INTO users (id, username, email, display_name, password_hash, avatar_url)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at`

	err := r.pool.QueryRow(
		ctx,
		query,
		user.ID,
		user.Username,
		user.Email,
		user.DisplayName,
		user.PasswordHash,
		user.AvatarURL,
	).Scan(&user.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			switch pgErr.ConstraintName {
			case "users_username_key":
				return domain.NewError(domain.ErrConflict, "USERNAME_TAKEN", "username is already taken")
			case "users_email_key":
				return domain.NewError(domain.ErrConflict, "EMAIL_TAKEN", "email is already registered")
			default:
				return domain.NewError(domain.ErrConflict, "CONFLICT", "resource already exists")
			}
		}
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

// GetByID fetches a user by primary key.
func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	const query = `
		SELECT id, username, email, display_name, password_hash, avatar_url, created_at
		FROM users
		WHERE id = $1`

	return r.scanOne(ctx, query, id)
}

// GetByLogin fetches a user matching either username or email (case-insensitive via CITEXT).
func (r *UserRepository) GetByLogin(ctx context.Context, login string) (*domain.User, error) {
	const query = `
		SELECT id, username, email, display_name, password_hash, avatar_url, created_at
		FROM users
		WHERE username = $1 OR email = $1
		LIMIT 1`

	return r.scanOne(ctx, query, login)
}

// Update applies partial updates to a user's profile and returns the updated row.
func (r *UserRepository) Update(ctx context.Context, id uuid.UUID, params domain.UpdateUserParams) (*domain.User, error) {
	setClauses := make([]string, 0, 2)
	args := make([]any, 0, 3)

	if params.DisplayName != nil {
		args = append(args, *params.DisplayName)
		setClauses = append(setClauses, fmt.Sprintf("display_name = $%d", len(args)))
	}
	if params.AvatarURLSet {
		args = append(args, params.AvatarURL)
		setClauses = append(setClauses, fmt.Sprintf("avatar_url = $%d", len(args)))
	}

	if len(setClauses) == 0 {
		return r.GetByID(ctx, id)
	}

	args = append(args, id)
	query := fmt.Sprintf(`
		UPDATE users
		SET %s
		WHERE id = $%d
		RETURNING id, username, email, display_name, password_hash, avatar_url, created_at`,
		strings.Join(setClauses, ", "),
		len(args),
	)

	return r.scanOne(ctx, query, args...)
}

func (r *UserRepository) scanOne(ctx context.Context, query string, args ...any) (*domain.User, error) {
	var u domain.User
	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&u.ID,
		&u.Username,
		&u.Email,
		&u.DisplayName,
		&u.PasswordHash,
		&u.AvatarURL,
		&u.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewError(domain.ErrNotFound, "USER_NOT_FOUND", "user not found")
		}
		return nil, fmt.Errorf("query user: %w", err)
	}
	return &u, nil
}
