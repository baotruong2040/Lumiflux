package gateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

const hookOperationTimeout = 3 * time.Second

// RealtimeCoordinator handles presence lifecycle, heartbeat renewal, and TYPING_START throttling.
type RealtimeCoordinator interface {
	HandleUserOnline(ctx context.Context, userID uuid.UUID, guildIDs []uuid.UUID)
	HandleUserOffline(ctx context.Context, userID uuid.UUID, guildIDs []uuid.UUID)
	HandleHeartbeat(ctx context.Context, userID uuid.UUID)
	HandleTypingStart(ctx context.Context, userID, channelID uuid.UUID) error
}

// BindRealtimeHooks connects Hub session/frame hooks to rt.
func BindRealtimeHooks(hub *Hub, rt RealtimeCoordinator, log *slog.Logger) {
	if hub == nil || rt == nil {
		return
	}
	if log == nil {
		log = slog.Default()
	}

	hub.SetOnlineHook(func(userID uuid.UUID, guildIDs []uuid.UUID) {
		ctx, cancel := context.WithTimeout(context.Background(), hookOperationTimeout)
		defer cancel()
		rt.HandleUserOnline(ctx, userID, guildIDs)
	})

	hub.SetOfflineHook(func(userID uuid.UUID, guildIDs []uuid.UUID) {
		ctx, cancel := context.WithTimeout(context.Background(), hookOperationTimeout)
		defer cancel()
		rt.HandleUserOffline(ctx, userID, guildIDs)
	})

	hub.SetHeartbeatHook(func(userID uuid.UUID) {
		ctx, cancel := context.WithTimeout(context.Background(), hookOperationTimeout)
		defer cancel()
		rt.HandleHeartbeat(ctx, userID)
	})

	hub.SetTypingHook(func(client *Client, data json.RawMessage) {
		var payload TypingStartPayload
		if err := json.Unmarshal(data, &payload); err != nil || payload.ChannelID == uuid.Nil {
			hub.SendError(client, "VALIDATION_ERROR", "channelId must be a valid UUID")
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), hookOperationTimeout)
		defer cancel()
		if err := rt.HandleTypingStart(ctx, client.UserID(), payload.ChannelID); err != nil {
			log.Debug("ignored typing start error", "user_id", client.UserID(), "channel_id", payload.ChannelID, "err", err)
		}
	})
}
