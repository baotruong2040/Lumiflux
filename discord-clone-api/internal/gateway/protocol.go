package gateway

import (
	"encoding/json"

	"github.com/google/uuid"
)

const (
	// SendBufferSize is the outbound per-client frame queue capacity (docs/architecture.md).
	SendBufferSize = 256

	// CloseNormalClosure indicates a standard WebSocket closure.
	CloseNormalClosure = 1000
	// CloseGoingAway indicates the server is shutting down.
	CloseGoingAway = 1001
	// CloseSlowConsumer indicates the client failed to drain its send queue in time.
	CloseSlowConsumer = 4002
	// CloseHeartbeatTimeout indicates the client detected missing HEARTBEAT_ACK frames.
	CloseHeartbeatTimeout = 4003
)

// WebSocket event names matching docs/architecture.md section 5.
const (
	EventReady             = "READY"
	EventHeartbeat         = "HEARTBEAT"
	EventHeartbeatAck      = "HEARTBEAT_ACK"
	EventTypingStart       = "TYPING_START"
	EventUserTyping        = "USER_TYPING"
	EventError             = "ERROR"
	EventMessageCreate     = "MESSAGE_CREATE"
	EventMessageUpdate     = "MESSAGE_UPDATE"
	EventMessageDelete     = "MESSAGE_DELETE"
	EventPresenceUpdate    = "PRESENCE_UPDATE"
	EventGuildMemberAdd    = "GUILD_MEMBER_ADD"
	EventGuildMemberRemove = "GUILD_MEMBER_REMOVE"
	EventGuildDelete       = "GUILD_DELETE"
	EventChannelCreate     = "CHANNEL_CREATE"
	EventChannelUpdate     = "CHANNEL_UPDATE"
	EventChannelDelete     = "CHANNEL_DELETE"
)

// InboundFrame represents a JSON event envelope sent from client to server.
type InboundFrame struct {
	T string          `json:"t"`
	D json.RawMessage `json:"d,omitempty"`
}

// OutboundFrame represents a JSON event envelope sent from server to client.
type OutboundFrame struct {
	T string `json:"t"`
	D any    `json:"d,omitempty"`
}

// ReadyPayload is dispatched immediately upon successful WebSocket registration.
type ReadyPayload struct {
	UserID              uuid.UUID   `json:"userId"`
	GuildIDs            []uuid.UUID `json:"guildIds"`
	HeartbeatIntervalMs int64       `json:"heartbeatIntervalMs"`
}

// ErrorPayload describes a protocol-level error frame sent to a connected client.
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// MarshalFrame encodes a WebSocket envelope frame as JSON bytes.
func MarshalFrame(eventType string, payload any) ([]byte, error) {
	return json.Marshal(OutboundFrame{
		T: eventType,
		D: payload,
	})
}
