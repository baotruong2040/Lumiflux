package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/truong/discord-clone-api/internal/domain"
)

// MessageRepository implements domain.MessageRepository using PostgreSQL via pgxpool.
type MessageRepository struct {
	pool *pgxpool.Pool
}

// NewMessageRepository constructs a MessageRepository backed by pool.
func NewMessageRepository(pool *pgxpool.Pool) *MessageRepository {
	return &MessageRepository{pool: pool}
}

// Create inserts a message and returns the persisted record joined with its author profile.
func (r *MessageRepository) Create(ctx context.Context, msg *domain.Message) (*domain.Message, error) {
	const query = `
		WITH inserted AS (
			INSERT INTO messages (id, channel_id, author_id, content)
			VALUES ($1, $2, $3, $4)
			RETURNING id, channel_id, author_id, content, edited_at, deleted_at, created_at
		)
		SELECT
			i.id, i.channel_id, i.author_id, i.content, i.edited_at, i.deleted_at, i.created_at,
			u.id, u.username, u.display_name, u.avatar_url
		FROM inserted i
		INNER JOIN users u ON u.id = i.author_id`

	created, err := r.scanMessage(r.pool.QueryRow(ctx, query, msg.ID, msg.ChannelID, msg.AuthorID, msg.Content))
	if err != nil {
		return nil, fmt.Errorf("insert message: %w", err)
	}
	created.Nonce = msg.Nonce
	return created, nil
}

// ListByChannel returns up to limit non-deleted messages ordered by UUIDv7 descending.
func (r *MessageRepository) ListByChannel(
	ctx context.Context,
	channelID uuid.UUID,
	before *uuid.UUID,
	limit int,
) ([]domain.Message, error) {
	var (
		rows pgx.Rows
		err  error
	)

	if before != nil {
		const queryWithCursor = `
			SELECT
				m.id, m.channel_id, m.author_id, m.content, m.edited_at, m.deleted_at, m.created_at,
				u.id, u.username, u.display_name, u.avatar_url
			FROM messages m
			INNER JOIN users u ON u.id = m.author_id
			WHERE m.channel_id = $1 AND m.id < $2 AND m.deleted_at IS NULL
			ORDER BY m.id DESC
			LIMIT $3`
		rows, err = r.pool.Query(ctx, queryWithCursor, channelID, *before, limit)
	} else {
		const queryFirstPage = `
			SELECT
				m.id, m.channel_id, m.author_id, m.content, m.edited_at, m.deleted_at, m.created_at,
				u.id, u.username, u.display_name, u.avatar_url
			FROM messages m
			INNER JOIN users u ON u.id = m.author_id
			WHERE m.channel_id = $1 AND m.deleted_at IS NULL
			ORDER BY m.id DESC
			LIMIT $2`
		rows, err = r.pool.Query(ctx, queryFirstPage, channelID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("list channel messages: %w", err)
	}
	defer rows.Close()

	messages := make([]domain.Message, 0)
	for rows.Next() {
		m, scanErr := r.scanMessage(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan message row: %w", scanErr)
		}
		messages = append(messages, *m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}
	return messages, nil
}

// GetByID returns an active (non-deleted) message joined with its author profile.
func (r *MessageRepository) GetByID(ctx context.Context, messageID uuid.UUID) (*domain.Message, error) {
	const query = `
		SELECT
			m.id, m.channel_id, m.author_id, m.content, m.edited_at, m.deleted_at, m.created_at,
			u.id, u.username, u.display_name, u.avatar_url
		FROM messages m
		INNER JOIN users u ON u.id = m.author_id
		WHERE m.id = $1 AND m.deleted_at IS NULL`

	msg, err := r.scanMessage(r.pool.QueryRow(ctx, query, messageID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewError(domain.ErrNotFound, "MESSAGE_NOT_FOUND", "message not found")
		}
		return nil, fmt.Errorf("get message: %w", err)
	}
	return msg, nil
}

// UpdateContent updates a non-deleted message's content and stamps edited_at.
func (r *MessageRepository) UpdateContent(ctx context.Context, messageID uuid.UUID, content string) (*domain.Message, error) {
	const query = `
		WITH updated AS (
			UPDATE messages
			SET content = $1, edited_at = now()
			WHERE id = $2 AND deleted_at IS NULL
			RETURNING id, channel_id, author_id, content, edited_at, deleted_at, created_at
		)
		SELECT
			up.id, up.channel_id, up.author_id, up.content, up.edited_at, up.deleted_at, up.created_at,
			u.id, u.username, u.display_name, u.avatar_url
		FROM updated up
		INNER JOIN users u ON u.id = up.author_id`

	msg, err := r.scanMessage(r.pool.QueryRow(ctx, query, content, messageID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewError(domain.ErrNotFound, "MESSAGE_NOT_FOUND", "message not found")
		}
		return nil, fmt.Errorf("update message: %w", err)
	}
	return msg, nil
}

// SoftDelete marks a message as deleted by setting deleted_at = now().
func (r *MessageRepository) SoftDelete(ctx context.Context, messageID uuid.UUID) error {
	const query = `UPDATE messages SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`

	tag, err := r.pool.Exec(ctx, query, messageID)
	if err != nil {
		return fmt.Errorf("soft delete message: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NewError(domain.ErrNotFound, "MESSAGE_NOT_FOUND", "message not found")
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func (r *MessageRepository) scanMessage(row rowScanner) (*domain.Message, error) {
	var m domain.Message
	err := row.Scan(
		&m.ID,
		&m.ChannelID,
		&m.AuthorID,
		&m.Content,
		&m.EditedAt,
		&m.DeletedAt,
		&m.CreatedAt,
		&m.Author.ID,
		&m.Author.Username,
		&m.Author.DisplayName,
		&m.Author.AvatarURL,
	)
	if err != nil {
		return nil, err
	}
	return &m, nil
}
