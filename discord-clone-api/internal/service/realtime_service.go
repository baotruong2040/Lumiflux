package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/truong/discord-clone-api/internal/domain"
	"github.com/truong/discord-clone-api/pkg/logger"
)

const (
	defaultPresenceTTL = 60 * time.Second
	defaultTypingTTL   = 3 * time.Second
)

// RealtimeService coordinates ephemeral online presence (presence:{userID}) and typing indicators (typing:{channelID}:{userID}).
type RealtimeService struct {
	guilds      domain.GuildRepository
	channels    domain.ChannelRepository
	presence    domain.PresenceStore
	typing      domain.TypingLimiter
	presencePub domain.PresenceEventPublisher
	typingPub   domain.TypingEventPublisher
	presenceTTL time.Duration
	typingTTL   time.Duration
	now         func() time.Time
}

// NewRealtimeService constructs a RealtimeService with safe defaults for TTLs and optional publishers.
func NewRealtimeService(
	guilds domain.GuildRepository,
	channels domain.ChannelRepository,
	presence domain.PresenceStore,
	typing domain.TypingLimiter,
	presencePub domain.PresenceEventPublisher,
	typingPub domain.TypingEventPublisher,
	presenceTTL time.Duration,
	typingTTL time.Duration,
) *RealtimeService {
	if presenceTTL <= 0 {
		presenceTTL = defaultPresenceTTL
	}
	if typingTTL <= 0 {
		typingTTL = defaultTypingTTL
	}
	if presencePub == nil {
		presencePub = domain.NoopPresenceEventPublisher{}
	}
	if typingPub == nil {
		typingPub = domain.NoopTypingEventPublisher{}
	}
	return &RealtimeService{
		guilds:      guilds,
		channels:    channels,
		presence:    presence,
		typing:      typing,
		presencePub: presencePub,
		typingPub:   typingPub,
		presenceTTL: presenceTTL,
		typingTTL:   typingTTL,
		now:         func() time.Time { return time.Now().UTC() },
	}
}

// HandleUserOnline marks userID online in Redis and broadcasts PRESENCE_UPDATE=online to guildIDs.
func (s *RealtimeService) HandleUserOnline(ctx context.Context, userID uuid.UUID, guildIDs []uuid.UUID) {
	if userID == uuid.Nil {
		return
	}
	if s.presence != nil {
		if err := s.presence.SetOnline(ctx, userID, s.presenceTTL); err != nil {
			logger.FromContext(ctx).Warn("failed to set user online presence", "user_id", userID, "err", err)
		}
	}
	s.presencePub.PublishPresenceUpdate(ctx, guildIDs, userID, true)
}

// HandleHeartbeat extends the TTL on userID's online presence key.
func (s *RealtimeService) HandleHeartbeat(ctx context.Context, userID uuid.UUID) {
	if userID == uuid.Nil || s.presence == nil {
		return
	}
	if err := s.presence.SetOnline(ctx, userID, s.presenceTTL); err != nil {
		logger.FromContext(ctx).Warn("failed to refresh presence ttl on heartbeat", "user_id", userID, "err", err)
	}
}

// HandleUserOffline clears userID's presence key and broadcasts PRESENCE_UPDATE=offline to guildIDs.
func (s *RealtimeService) HandleUserOffline(ctx context.Context, userID uuid.UUID, guildIDs []uuid.UUID) {
	if userID == uuid.Nil {
		return
	}
	if s.presence != nil {
		if err := s.presence.SetOffline(ctx, userID); err != nil {
			logger.FromContext(ctx).Warn("failed to delete presence key on disconnect", "user_id", userID, "err", err)
		}
	}
	s.presencePub.PublishPresenceUpdate(ctx, guildIDs, userID, false)
}

// HandleTypingStart verifies channel membership, enforces SET NX EX 3 throttle, and publishes USER_TYPING.
func (s *RealtimeService) HandleTypingStart(ctx context.Context, userID, channelID uuid.UUID) error {
	if userID == uuid.Nil || channelID == uuid.Nil {
		return domain.NewError(domain.ErrValidation, "VALIDATION_ERROR", "invalid typing payload")
	}

	ch, err := s.channels.GetByID(ctx, channelID)
	if err != nil {
		return err
	}
	_, member, err := loadGuildAndMember(ctx, s.guilds, userID, ch.GuildID)
	if err != nil {
		return err
	}

	if s.typing != nil {
		allowed, err := s.typing.AllowTyping(ctx, channelID, userID, s.typingTTL)
		if err != nil {
			logger.FromContext(ctx).Warn("typing limiter error, skipping typing broadcast", "channel_id", channelID, "user_id", userID, "err", err)
			return nil
		}
		if !allowed {
			return nil
		}
	}

	username := member.User.Username
	if username == "" {
		username = member.User.DisplayName
	}
	s.typingPub.PublishUserTyping(ctx, ch.GuildID, channelID, userID, username, s.now().UnixMilli())
	return nil
}
