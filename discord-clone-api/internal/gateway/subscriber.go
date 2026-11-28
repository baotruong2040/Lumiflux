package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	guildChannelPrefix  = "guild:"
	guildChannelPattern = "guild:*"
)

// Subscriber consumes Redis Pub/Sub guild events and dispatches them into Hub (ADR-003).
type Subscriber struct {
	client *redis.Client
	hub    *Hub
	log    *slog.Logger
}

// NewSubscriber constructs a Redis Pub/Sub Subscriber targeting hub.
func NewSubscriber(client *redis.Client, hub *Hub, log *slog.Logger) *Subscriber {
	if log == nil {
		log = slog.Default()
	}
	return &Subscriber{
		client: client,
		hub:    hub,
		log:    log,
	}
}

// Start subscribes to guild:* on Redis and forwards messages to Hub until ctx is cancelled.
// The returned channel closes once the subscriber goroutine has exited cleanly.
func (s *Subscriber) Start(ctx context.Context) (<-chan struct{}, error) {
	pubsub := s.client.PSubscribe(ctx, guildChannelPattern)
	if _, err := pubsub.Receive(ctx); err != nil {
		_ = pubsub.Close()
		return nil, fmt.Errorf("subscribe %s: %w", guildChannelPattern, err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			_ = pubsub.Close()
		}()

		stopClose := context.AfterFunc(ctx, func() {
			_ = pubsub.Close()
		})
		defer stopClose()

		msgCh := pubsub.Channel()
		for msg := range msgCh {
			s.dispatchMessage(ctx, msg.Channel, []byte(msg.Payload))
		}
	}()

	return done, nil
}

type guildUserEventPayload struct {
	User struct {
		ID uuid.UUID `json:"id"`
	} `json:"user"`
}

func (s *Subscriber) dispatchMessage(ctx context.Context, channel string, payload []byte) {
	rawGuildID, ok := strings.CutPrefix(channel, guildChannelPrefix)
	if !ok {
		return
	}
	guildID, err := uuid.Parse(rawGuildID)
	if err != nil {
		s.log.Warn("ignoring invalid guild pubsub channel", "channel", channel, "err", err)
		return
	}

	var env InboundFrame
	if err := json.Unmarshal(payload, &env); err != nil {
		s.log.Warn("ignoring malformed pubsub event frame", "channel", channel, "err", err)
		return
	}

	switch env.T {
	case EventGuildDelete:
		s.hub.BroadcastAndRemoveGuild(ctx, guildID, payload)
	case EventGuildMemberRemove:
		if uid := parseGuildEventUserID(env.D); uid != uuid.Nil {
			s.hub.UnsubscribeAndBroadcast(ctx, uid, guildID, payload)
			return
		}
		s.hub.BroadcastToGuild(ctx, guildID, payload)
	case EventGuildMemberAdd:
		if uid := parseGuildEventUserID(env.D); uid != uuid.Nil {
			s.hub.SubscribeAndBroadcast(ctx, uid, guildID, payload)
			return
		}
		s.hub.BroadcastToGuild(ctx, guildID, payload)
	default:
		s.hub.BroadcastToGuild(ctx, guildID, payload)
	}
}

func parseGuildEventUserID(raw json.RawMessage) uuid.UUID {
	var body guildUserEventPayload
	if err := json.Unmarshal(raw, &body); err != nil {
		return uuid.Nil
	}
	return body.User.ID
}
