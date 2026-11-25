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

// ChannelRepository implements domain.ChannelRepository using PostgreSQL via pgxpool.
type ChannelRepository struct {
	pool *pgxpool.Pool
}

// NewChannelRepository constructs a ChannelRepository backed by pool.
func NewChannelRepository(pool *pgxpool.Pool) *ChannelRepository {
	return &ChannelRepository{pool: pool}
}

// Create inserts a channel with the next guild position and populates Position and CreatedAt.
func (r *ChannelRepository) Create(ctx context.Context, channel *domain.Channel) error {
	const query = `
		INSERT INTO channels (id, guild_id, name, type, topic, position)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			COALESCE((SELECT MAX(position) + 1 FROM channels WHERE guild_id = $2), 0)
		)
		RETURNING position, created_at`

	err := r.pool.QueryRow(
		ctx,
		query,
		channel.ID,
		channel.GuildID,
		channel.Name,
		channel.Type,
		channel.Topic,
	).Scan(&channel.Position, &channel.CreatedAt)
	if err != nil {
		if mapped := mapChannelPgError(err); mapped != nil {
			return mapped
		}
		return fmt.Errorf("insert channel: %w", err)
	}
	return nil
}

// ListByGuild returns all channels belonging to guildID ordered by position and creation order.
func (r *ChannelRepository) ListByGuild(ctx context.Context, guildID uuid.UUID) ([]domain.Channel, error) {
	const query = `
		SELECT id, guild_id, name, type, topic, position, created_at
		FROM channels
		WHERE guild_id = $1
		ORDER BY position ASC, created_at ASC, id ASC`

	rows, err := r.pool.Query(ctx, query, guildID)
	if err != nil {
		return nil, fmt.Errorf("list channels by guild: %w", err)
	}
	defer rows.Close()

	channels := make([]domain.Channel, 0)
	for rows.Next() {
		var ch domain.Channel
		if err := rows.Scan(
			&ch.ID,
			&ch.GuildID,
			&ch.Name,
			&ch.Type,
			&ch.Topic,
			&ch.Position,
			&ch.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan channel row: %w", err)
		}
		channels = append(channels, ch)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate channels: %w", err)
	}
	return channels, nil
}

// GetByID fetches a channel by its primary key.
func (r *ChannelRepository) GetByID(ctx context.Context, channelID uuid.UUID) (*domain.Channel, error) {
	const query = `
		SELECT id, guild_id, name, type, topic, position, created_at
		FROM channels
		WHERE id = $1`

	return r.scanChannel(ctx, query, channelID)
}

// Update applies partial updates to a channel and returns the updated record.
func (r *ChannelRepository) Update(ctx context.Context, channelID uuid.UUID, params domain.UpdateChannelParams) (*domain.Channel, error) {
	setClauses := make([]string, 0, 2)
	args := make([]any, 0, 3)

	if params.Name != nil {
		args = append(args, *params.Name)
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", len(args)))
	}
	if params.TopicSet {
		args = append(args, params.Topic)
		setClauses = append(setClauses, fmt.Sprintf("topic = $%d", len(args)))
	}

	if len(setClauses) == 0 {
		return r.GetByID(ctx, channelID)
	}

	args = append(args, channelID)
	query := fmt.Sprintf(`
		UPDATE channels
		SET %s
		WHERE id = $%d
		RETURNING id, guild_id, name, type, topic, position, created_at`,
		strings.Join(setClauses, ", "),
		len(args),
	)

	return r.scanChannel(ctx, query, args...)
}

// Delete removes a channel by ID.
func (r *ChannelRepository) Delete(ctx context.Context, channelID uuid.UUID) error {
	const query = `DELETE FROM channels WHERE id = $1`

	tag, err := r.pool.Exec(ctx, query, channelID)
	if err != nil {
		return fmt.Errorf("delete channel: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NewError(domain.ErrNotFound, "CHANNEL_NOT_FOUND", "channel not found")
	}
	return nil
}

func (r *ChannelRepository) scanChannel(ctx context.Context, query string, args ...any) (*domain.Channel, error) {
	var ch domain.Channel
	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&ch.ID,
		&ch.GuildID,
		&ch.Name,
		&ch.Type,
		&ch.Topic,
		&ch.Position,
		&ch.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewError(domain.ErrNotFound, "CHANNEL_NOT_FOUND", "channel not found")
		}
		if mapped := mapChannelPgError(err); mapped != nil {
			return nil, mapped
		}
		return nil, fmt.Errorf("query channel: %w", err)
	}
	return &ch, nil
}

func mapChannelPgError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return domain.NewError(domain.ErrConflict, "CHANNEL_NAME_TAKEN", "channel name already exists in this guild")
	}
	return nil
}
