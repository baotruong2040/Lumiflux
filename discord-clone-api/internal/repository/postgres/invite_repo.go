package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/truong/discord-clone-api/internal/domain"
)

// InviteRepository implements domain.InviteRepository using PostgreSQL via pgxpool.
type InviteRepository struct {
	pool *pgxpool.Pool
}

// NewInviteRepository constructs an InviteRepository backed by pool.
func NewInviteRepository(pool *pgxpool.Pool) *InviteRepository {
	return &InviteRepository{pool: pool}
}

// Create inserts a new invite row and populates CreatedAt from the database.
func (r *InviteRepository) Create(ctx context.Context, invite *domain.Invite) error {
	const query = `
		INSERT INTO invites (code, guild_id, created_by, max_uses, uses, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at`

	err := r.pool.QueryRow(
		ctx,
		query,
		invite.Code,
		invite.GuildID,
		invite.CreatedBy,
		invite.MaxUses,
		invite.Uses,
		invite.ExpiresAt,
	).Scan(&invite.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.NewError(domain.ErrConflict, "INVITE_CODE_CONFLICT", "invite code already exists")
		}
		return fmt.Errorf("insert invite: %w", err)
	}
	return nil
}

// Accept validates and consumes an invite code in a single transaction, adding userID as a member.
func (r *InviteRepository) Accept(ctx context.Context, code string, userID uuid.UUID, now time.Time) (*domain.Guild, *domain.GuildMember, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("begin accept invite tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	const selectInvite = `
		SELECT code, guild_id, created_by, max_uses, uses, expires_at, created_at
		FROM invites
		WHERE code = $1
		FOR UPDATE`

	var inv domain.Invite
	err = tx.QueryRow(ctx, selectInvite, code).Scan(
		&inv.Code,
		&inv.GuildID,
		&inv.CreatedBy,
		&inv.MaxUses,
		&inv.Uses,
		&inv.ExpiresAt,
		&inv.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, domain.NewError(domain.ErrNotFound, "INVITE_NOT_FOUND", "invite code not found")
		}
		return nil, nil, fmt.Errorf("lock invite: %w", err)
	}

	if inv.ExpiresAt != nil && !inv.ExpiresAt.After(now) {
		return nil, nil, domain.NewError(domain.ErrValidation, "INVITE_EXPIRED", "invite code has expired")
	}

	if inv.MaxUses != nil && inv.Uses >= *inv.MaxUses {
		return nil, nil, domain.NewError(domain.ErrValidation, "INVITE_EXHAUSTED", "invite code has reached its maximum uses")
	}

	const checkMember = `SELECT 1 FROM guild_members WHERE guild_id = $1 AND user_id = $2`
	var exists int
	err = tx.QueryRow(ctx, checkMember, inv.GuildID, userID).Scan(&exists)
	if err == nil {
		return nil, nil, domain.NewError(domain.ErrConflict, "ALREADY_MEMBER", "you are already a member of this guild")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, fmt.Errorf("check existing guild member: %w", err)
	}

	const incrementUses = `UPDATE invites SET uses = uses + 1 WHERE code = $1`
	if _, err := tx.Exec(ctx, incrementUses, code); err != nil {
		return nil, nil, fmt.Errorf("increment invite uses: %w", err)
	}

	member := domain.GuildMember{
		GuildID: inv.GuildID,
		UserID:  userID,
		Role:    domain.RoleMember,
	}
	const insertMember = `
		INSERT INTO guild_members (guild_id, user_id, role)
		VALUES ($1, $2, $3)
		RETURNING joined_at`
	if err := tx.QueryRow(ctx, insertMember, member.GuildID, member.UserID, string(member.Role)).Scan(&member.JoinedAt); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, nil, domain.NewError(domain.ErrConflict, "ALREADY_MEMBER", "you are already a member of this guild")
		}
		return nil, nil, fmt.Errorf("insert invited guild member: %w", err)
	}

	const selectGuild = `
		SELECT id, name, icon_url, owner_id, created_at
		FROM guilds
		WHERE id = $1`
	var guild domain.Guild
	if err := tx.QueryRow(ctx, selectGuild, inv.GuildID).Scan(
		&guild.ID,
		&guild.Name,
		&guild.IconURL,
		&guild.OwnerID,
		&guild.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, domain.NewError(domain.ErrNotFound, "GUILD_NOT_FOUND", "guild not found")
		}
		return nil, nil, fmt.Errorf("load guild after invite accept: %w", err)
	}

	const selectUser = `
		SELECT id, username, email, display_name, avatar_url, created_at
		FROM users
		WHERE id = $1`
	if err := tx.QueryRow(ctx, selectUser, userID).Scan(
		&member.User.ID,
		&member.User.Username,
		&member.User.Email,
		&member.User.DisplayName,
		&member.User.AvatarURL,
		&member.User.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, domain.NewError(domain.ErrNotFound, "USER_NOT_FOUND", "user not found")
		}
		return nil, nil, fmt.Errorf("load user after invite accept: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, fmt.Errorf("commit accept invite tx: %w", err)
	}

	return &guild, &member, nil
}
