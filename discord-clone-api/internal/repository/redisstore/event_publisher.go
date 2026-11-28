package redisstore

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/truong/discord-clone-api/internal/domain"
)

// GuildPubSubPrefix is prepended to guild UUIDs to form Redis Pub/Sub channels.
const GuildPubSubPrefix = "guild:"

const publishTimeout = 2 * time.Second

// GuildPubSubChannel returns the Redis Pub/Sub channel name for guildID.
func GuildPubSubChannel(guildID uuid.UUID) string {
	return GuildPubSubPrefix + guildID.String()
}

// EventPublisher publishes realtime domain events to Redis Pub/Sub guild channels (ADR-003).
type EventPublisher struct {
	client *redis.Client
	log    *slog.Logger
}

var (
	_ domain.MessageEventPublisher  = (*EventPublisher)(nil)
	_ domain.ChannelEventPublisher  = (*EventPublisher)(nil)
	_ domain.GuildEventPublisher    = (*EventPublisher)(nil)
	_ domain.PresenceEventPublisher = (*EventPublisher)(nil)
	_ domain.TypingEventPublisher   = (*EventPublisher)(nil)
)

// NewEventPublisher constructs a Redis Pub/Sub EventPublisher.
func NewEventPublisher(client *redis.Client, log *slog.Logger) *EventPublisher {
	if log == nil {
		log = slog.Default()
	}
	return &EventPublisher{
		client: client,
		log:    log,
	}
}

type eventEnvelope struct {
	T string `json:"t"`
	D any    `json:"d"`
}

type eventPublicUser struct {
	ID          uuid.UUID `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"displayName"`
	AvatarURL   *string   `json:"avatarUrl"`
}

type eventChannelPayload struct {
	ID        uuid.UUID `json:"id"`
	GuildID   uuid.UUID `json:"guildId"`
	Name      string    `json:"name"`
	Type      int16     `json:"type"`
	Topic     *string   `json:"topic"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"createdAt"`
}

type messageCreateEventData struct {
	ID        uuid.UUID       `json:"id"`
	ChannelID uuid.UUID       `json:"channelId"`
	GuildID   uuid.UUID       `json:"guildId"`
	Author    eventPublicUser `json:"author"`
	Content   string          `json:"content"`
	Nonce     *string         `json:"nonce"`
	EditedAt  *time.Time      `json:"editedAt"`
	CreatedAt time.Time       `json:"createdAt"`
}

type messageUpdateEventData struct {
	ID        uuid.UUID  `json:"id"`
	ChannelID uuid.UUID  `json:"channelId"`
	GuildID   uuid.UUID  `json:"guildId"`
	Content   string     `json:"content"`
	EditedAt  *time.Time `json:"editedAt"`
}

type messageDeleteEventData struct {
	ID        uuid.UUID `json:"id"`
	ChannelID uuid.UUID `json:"channelId"`
	GuildID   uuid.UUID `json:"guildId"`
}

type channelEventData struct {
	GuildID uuid.UUID           `json:"guildId"`
	Channel eventChannelPayload `json:"channel"`
}

type guildMemberAddEventData struct {
	GuildID  uuid.UUID       `json:"guildId"`
	User     eventPublicUser `json:"user"`
	Role     string          `json:"role"`
	Nickname *string         `json:"nickname"`
	JoinedAt time.Time       `json:"joinedAt"`
}

type guildMemberRemoveEventData struct {
	GuildID uuid.UUID       `json:"guildId"`
	User    eventPublicUser `json:"user"`
}

type guildDeleteEventData struct {
	GuildID uuid.UUID `json:"guildId"`
}

type presenceUpdateEventData struct {
	UserID uuid.UUID `json:"userId"`
	Status string    `json:"status"`
}

type userTypingEventData struct {
	ChannelID uuid.UUID `json:"channelId"`
	GuildID   uuid.UUID `json:"guildId"`
	UserID    uuid.UUID `json:"userId"`
	Username  string    `json:"username"`
	Timestamp int64     `json:"timestamp"`
}

// PublishMessageCreate broadcasts MESSAGE_CREATE on guild:{guildID}.
func (p *EventPublisher) PublishMessageCreate(ctx context.Context, guildID uuid.UUID, msg domain.Message) {
	p.publish(ctx, guildID, "MESSAGE_CREATE", messageCreateEventData{
		ID:        msg.ID,
		ChannelID: msg.ChannelID,
		GuildID:   guildID,
		Author: eventPublicUser{
			ID:          msg.Author.ID,
			Username:    msg.Author.Username,
			DisplayName: msg.Author.DisplayName,
			AvatarURL:   msg.Author.AvatarURL,
		},
		Content:   msg.Content,
		Nonce:     msg.Nonce,
		EditedAt:  msg.EditedAt,
		CreatedAt: msg.CreatedAt,
	})
}

// PublishMessageUpdate broadcasts MESSAGE_UPDATE on guild:{guildID}.
func (p *EventPublisher) PublishMessageUpdate(ctx context.Context, guildID uuid.UUID, msg domain.Message) {
	p.publish(ctx, guildID, "MESSAGE_UPDATE", messageUpdateEventData{
		ID:        msg.ID,
		ChannelID: msg.ChannelID,
		GuildID:   guildID,
		Content:   msg.Content,
		EditedAt:  msg.EditedAt,
	})
}

// PublishMessageDelete broadcasts MESSAGE_DELETE on guild:{guildID}.
func (p *EventPublisher) PublishMessageDelete(ctx context.Context, guildID, channelID, messageID uuid.UUID) {
	p.publish(ctx, guildID, "MESSAGE_DELETE", messageDeleteEventData{
		ID:        messageID,
		ChannelID: channelID,
		GuildID:   guildID,
	})
}

// PublishChannelCreate broadcasts CHANNEL_CREATE on guild:{guildID}.
func (p *EventPublisher) PublishChannelCreate(ctx context.Context, guildID uuid.UUID, ch domain.Channel) {
	p.publish(ctx, guildID, "CHANNEL_CREATE", toChannelEventData(guildID, ch))
}

// PublishChannelUpdate broadcasts CHANNEL_UPDATE on guild:{guildID}.
func (p *EventPublisher) PublishChannelUpdate(ctx context.Context, guildID uuid.UUID, ch domain.Channel) {
	p.publish(ctx, guildID, "CHANNEL_UPDATE", toChannelEventData(guildID, ch))
}

// PublishChannelDelete broadcasts CHANNEL_DELETE on guild:{guildID}.
func (p *EventPublisher) PublishChannelDelete(ctx context.Context, guildID uuid.UUID, ch domain.Channel) {
	p.publish(ctx, guildID, "CHANNEL_DELETE", toChannelEventData(guildID, ch))
}

// PublishGuildMemberAdd broadcasts GUILD_MEMBER_ADD on guild:{guildID}.
func (p *EventPublisher) PublishGuildMemberAdd(ctx context.Context, guildID uuid.UUID, member domain.GuildMember) {
	user := member.User
	if user.ID == uuid.Nil {
		user.ID = member.UserID
	}
	p.publish(ctx, guildID, "GUILD_MEMBER_ADD", guildMemberAddEventData{
		GuildID: guildID,
		User: eventPublicUser{
			ID:          user.ID,
			Username:    user.Username,
			DisplayName: user.DisplayName,
			AvatarURL:   user.AvatarURL,
		},
		Role:     string(member.Role),
		Nickname: member.Nickname,
		JoinedAt: member.JoinedAt,
	})
}

// PublishGuildMemberRemove broadcasts GUILD_MEMBER_REMOVE on guild:{guildID}.
func (p *EventPublisher) PublishGuildMemberRemove(ctx context.Context, guildID uuid.UUID, user domain.User) {
	p.publish(ctx, guildID, "GUILD_MEMBER_REMOVE", guildMemberRemoveEventData{
		GuildID: guildID,
		User: eventPublicUser{
			ID:          user.ID,
			Username:    user.Username,
			DisplayName: user.DisplayName,
			AvatarURL:   user.AvatarURL,
		},
	})
}

// PublishGuildDelete broadcasts GUILD_DELETE on guild:{guildID}.
func (p *EventPublisher) PublishGuildDelete(ctx context.Context, guildID uuid.UUID) {
	p.publish(ctx, guildID, "GUILD_DELETE", guildDeleteEventData{
		GuildID: guildID,
	})
}

// PublishPresenceUpdate broadcasts PRESENCE_UPDATE across each unique guildID in guildIDs.
func (p *EventPublisher) PublishPresenceUpdate(ctx context.Context, guildIDs []uuid.UUID, userID uuid.UUID, online bool) {
	if len(guildIDs) == 0 || userID == uuid.Nil {
		return
	}
	status := "offline"
	if online {
		status = "online"
	}
	payload := presenceUpdateEventData{
		UserID: userID,
		Status: status,
	}
	seen := make(map[uuid.UUID]struct{}, len(guildIDs))
	for _, gid := range guildIDs {
		if gid == uuid.Nil {
			continue
		}
		if _, dup := seen[gid]; dup {
			continue
		}
		seen[gid] = struct{}{}
		p.publish(ctx, gid, "PRESENCE_UPDATE", payload)
	}
}

// PublishUserTyping broadcasts USER_TYPING on guild:{guildID}.
func (p *EventPublisher) PublishUserTyping(
	ctx context.Context,
	guildID, channelID, userID uuid.UUID,
	username string,
	timestamp int64,
) {
	p.publish(ctx, guildID, "USER_TYPING", userTypingEventData{
		ChannelID: channelID,
		GuildID:   guildID,
		UserID:    userID,
		Username:  username,
		Timestamp: timestamp,
	})
}

func toChannelEventData(guildID uuid.UUID, ch domain.Channel) channelEventData {
	return channelEventData{
		GuildID: guildID,
		Channel: eventChannelPayload{
			ID:        ch.ID,
			GuildID:   ch.GuildID,
			Name:      ch.Name,
			Type:      ch.Type,
			Topic:     ch.Topic,
			Position:  ch.Position,
			CreatedAt: ch.CreatedAt,
		},
	}
}

func (p *EventPublisher) publish(ctx context.Context, guildID uuid.UUID, eventType string, payload any) {
	if p == nil || p.client == nil || guildID == uuid.Nil {
		return
	}

	raw, err := json.Marshal(eventEnvelope{
		T: eventType,
		D: payload,
	})
	if err != nil {
		p.log.Warn("failed to marshal guild realtime event", "event", eventType, "guild_id", guildID, "err", err)
		return
	}

	baseCtx := ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	pubCtx, cancel := context.WithTimeout(context.WithoutCancel(baseCtx), publishTimeout)
	defer cancel()

	channel := GuildPubSubChannel(guildID)
	if err := p.client.Publish(pubCtx, channel, raw).Err(); err != nil {
		// ADR-003: at-most-once Pub/Sub failure does not abort already-committed DB mutations.
		p.log.Warn("failed to publish guild realtime event", "event", eventType, "channel", channel, "err", err)
	}
}
