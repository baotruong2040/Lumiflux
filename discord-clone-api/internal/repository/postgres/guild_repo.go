package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/truong/discord-clone-api/internal/domain"
)

// GuildRepository implements domain.GuildRepository using PostgreSQL via pgxpool.
type GuildRepository struct {
	pool *pgxpool.Pool
}

// NewGuildRepository constructs a GuildRepository backed by pool.
func NewGuildRepository(pool *pgxpool.Pool) *GuildRepository {
	return &GuildRepository{pool: pool}
}

// CreateWithDefaultChannel atomically creates a guild, its default #general channel, and the owner membership.
func (r *GuildRepository) CreateWithDefaultChannel(
	ctx context.Context,
	guild *domain.Guild,
	defaultChannel *domain.Channel,
	ownerMember *domain.GuildMember,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin create guild tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	const insertGuild = `
		INSERT INTO guilds (id, name, icon_url, owner_id)
		VALUES ($1, $2, $3, $4)
		RETURNING created_at`
	if err := tx.QueryRow(ctx, insertGuild, guild.ID, guild.Name, guild.IconURL, guild.OwnerID).Scan(&guild.CreatedAt); err != nil {
		return fmt.Errorf("insert guild: %w", err)
	}

	const insertChannel = `
		INSERT INTO channels (id, guild_id, name, type, topic, position)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at`
	if err := tx.QueryRow(
		ctx,
		insertChannel,
		defaultChannel.ID,
		defaultChannel.GuildID,
		defaultChannel.Name,
		defaultChannel.Type,
		defaultChannel.Topic,
		defaultChannel.Position,
	).Scan(&defaultChannel.CreatedAt); err != nil {
		return fmt.Errorf("insert default channel: %w", err)
	}

	const insertMember = `
		INSERT INTO guild_members (guild_id, user_id, nickname, role)
		VALUES ($1, $2, $3, $4)
		RETURNING joined_at`
	if err := tx.QueryRow(
		ctx,
		insertMember,
		ownerMember.GuildID,
		ownerMember.UserID,
		ownerMember.Nickname,
		string(ownerMember.Role),
	).Scan(&ownerMember.JoinedAt); err != nil {
		return fmt.Errorf("insert owner member: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit create guild tx: %w", err)
	}
	return nil
}

// ListByUser returns all guilds that userID belongs to, ordered by join time.
func (r *GuildRepository) ListByUser(ctx context.Context, userID uuid.UUID) ([]domain.Guild, error) {
	const query = `
		SELECT g.id, g.name, g.icon_url, g.owner_id, g.created_at
		FROM guilds g
		INNER JOIN guild_members gm ON gm.guild_id = g.id
		WHERE gm.user_id = $1
		ORDER BY gm.joined_at ASC, g.id ASC`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list guilds by user: %w", err)
	}
	defer rows.Close()

	guilds := make([]domain.Guild, 0)
	for rows.Next() {
		var g domain.Guild
		if err := rows.Scan(&g.ID, &g.Name, &g.IconURL, &g.OwnerID, &g.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan guild row: %w", err)
		}
		guilds = append(guilds, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate guilds: %w", err)
	}
	return guilds, nil
}

// GetByID fetches a guild by its primary key.
func (r *GuildRepository) GetByID(ctx context.Context, guildID uuid.UUID) (*domain.Guild, error) {
	const query = `
		SELECT id, name, icon_url, owner_id, created_at
		FROM guilds
		WHERE id = $1`

	return r.scanGuild(ctx, query, guildID)
}

// Update applies partial updates to a guild and returns the updated record.
func (r *GuildRepository) Update(ctx context.Context, guildID uuid.UUID, params domain.UpdateGuildParams) (*domain.Guild, error) {
	setClauses := make([]string, 0, 2)
	args := make([]any, 0, 3)

	if params.Name != nil {
		args = append(args, *params.Name)
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", len(args)))
	}
	if params.IconURLSet {
		args = append(args, params.IconURL)
		setClauses = append(setClauses, fmt.Sprintf("icon_url = $%d", len(args)))
	}

	if len(setClauses) == 0 {
		return r.GetByID(ctx, guildID)
	}

	args = append(args, guildID)
	query := fmt.Sprintf(`
		UPDATE guilds
		SET %s
		WHERE id = $%d
		RETURNING id, name, icon_url, owner_id, created_at`,
		strings.Join(setClauses, ", "),
		len(args),
	)

	return r.scanGuild(ctx, query, args...)
}

// Delete removes a guild by ID; cascade rules delete its channels, members, and invites.
func (r *GuildRepository) Delete(ctx context.Context, guildID uuid.UUID) error {
	const query = `DELETE FROM guilds WHERE id = $1`

	tag, err := r.pool.Exec(ctx, query, guildID)
	if err != nil {
		return fmt.Errorf("delete guild: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NewError(domain.ErrNotFound, "GUILD_NOT_FOUND", "guild not found")
	}
	return nil
}

// GetMember returns the membership record joined with user profile for userID in guildID.
func (r *GuildRepository) GetMember(ctx context.Context, guildID, userID uuid.UUID) (*domain.GuildMember, error) {
	const query = `
		SELECT
			gm.guild_id, gm.user_id, gm.nickname, gm.role, gm.joined_at,
			u.id, u.username, u.email, u.display_name, u.avatar_url, u.created_at
		FROM guild_members gm
		INNER JOIN users u ON u.id = gm.user_id
		WHERE gm.guild_id = $1 AND gm.user_id = $2`

	var m domain.GuildMember
	var roleStr string
	err := r.pool.QueryRow(ctx, query, guildID, userID).Scan(
		&m.GuildID,
		&m.UserID,
		&m.Nickname,
		&roleStr,
		&m.JoinedAt,
		&m.User.ID,
		&m.User.Username,
		&m.User.Email,
		&m.User.DisplayName,
		&m.User.AvatarURL,
		&m.User.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewError(domain.ErrNotFound, "MEMBER_NOT_FOUND", "member not found")
		}
		return nil, fmt.Errorf("get guild member: %w", err)
	}
	m.Role = domain.Role(roleStr)
	return &m, nil
}

// ListMembers returns all members of guildID joined with their user profiles.
func (r *GuildRepository) ListMembers(ctx context.Context, guildID uuid.UUID) ([]domain.GuildMember, error) {
	const query = `
		SELECT
			gm.guild_id, gm.user_id, gm.nickname, gm.role, gm.joined_at,
			u.id, u.username, u.email, u.display_name, u.avatar_url, u.created_at
		FROM guild_members gm
		INNER JOIN users u ON u.id = gm.user_id
		WHERE gm.guild_id = $1
		ORDER BY CASE gm.role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END, gm.joined_at ASC, u.id ASC`

	rows, err := r.pool.Query(ctx, query, guildID)
	if err != nil {
		return nil, fmt.Errorf("list guild members: %w", err)
	}
	defer rows.Close()

	members := make([]domain.GuildMember, 0)
	for rows.Next() {
		var m domain.GuildMember
		var roleStr string
		if err := rows.Scan(
			&m.GuildID,
			&m.UserID,
			&m.Nickname,
			&roleStr,
			&m.JoinedAt,
			&m.User.ID,
			&m.User.Username,
			&m.User.Email,
			&m.User.DisplayName,
			&m.User.AvatarURL,
			&m.User.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan guild member: %w", err)
		}
		m.Role = domain.Role(roleStr)
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate guild members: %w", err)
	}
	return members, nil
}

// RemoveMember removes a user's membership from a guild.
func (r *GuildRepository) RemoveMember(ctx context.Context, guildID, userID uuid.UUID) error {
	const query = `DELETE FROM guild_members WHERE guild_id = $1 AND user_id = $2`

	tag, err := r.pool.Exec(ctx, query, guildID, userID)
	if err != nil {
		return fmt.Errorf("remove guild member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NewError(domain.ErrNotFound, "MEMBER_NOT_FOUND", "member not found")
	}
	return nil
}

func (r *GuildRepository) scanGuild(ctx context.Context, query string, args ...any) (*domain.Guild, error) {
	var g domain.Guild
	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&g.ID,
		&g.Name,
		&g.IconURL,
		&g.OwnerID,
		&g.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewError(domain.ErrNotFound, "GUILD_NOT_FOUND", "guild not found")
		}
		return nil, fmt.Errorf("query guild: %w", err)
	}
	return &g, nil
}
