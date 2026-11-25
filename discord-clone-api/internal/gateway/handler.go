package gateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/truong/discord-clone-api/internal/domain"
)

// TicketConsumer atomically validates and consumes single-use WebSocket upgrade tickets.
type TicketConsumer interface {
	ConsumeTicket(ctx context.Context, ticket string) (uuid.UUID, error)
}

// GuildLister returns all guilds a user is currently a member of.
type GuildLister interface {
	ListByUser(ctx context.Context, userID uuid.UUID) ([]domain.Guild, error)
}

// Handler upgrades authenticated HTTP requests at GET /ws?ticket=... into Hub-managed WebSocket sessions.
type Handler struct {
	hub            *Hub
	tickets        TicketConsumer
	guilds         GuildLister
	allowedOrigins map[string]struct{}
	upgrader       websocket.Upgrader
	clientOpts     ClientOptions
	log            *slog.Logger
}

// NewHandler constructs a WebSocket upgrade Handler with strict origin checking.
func NewHandler(
	hub *Hub,
	tickets TicketConsumer,
	guilds GuildLister,
	allowedOrigins []string,
	log *slog.Logger,
) *Handler {
	if log == nil {
		log = slog.Default()
	}

	originSet := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		trimmed := strings.TrimSpace(origin)
		if trimmed != "" {
			originSet[trimmed] = struct{}{}
		}
	}

	h := &Handler{
		hub:            hub,
		tickets:        tickets,
		guilds:         guilds,
		allowedOrigins: originSet,
		clientOpts: ClientOptions{
			Logger: log,
		},
		log: log,
	}

	h.upgrader = websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin:     h.isOriginAllowed,
	}

	return h
}

// ServeGin adapts ServeHTTP for Gin routing.
func (h *Handler) ServeGin(c *gin.Context) {
	h.ServeHTTP(c.Writer, c.Request)
}

// ServeHTTP validates origin and single-use ticket (HTTP 401 if invalid/expired), loads memberships, and upgrades to WebSocket.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.isOriginAllowed(r) {
		writeJSONError(w, http.StatusForbidden, "FORBIDDEN", "origin not allowed")
		return
	}

	ticket := strings.TrimSpace(r.URL.Query().Get("ticket"))
	if ticket == "" {
		writeJSONError(w, http.StatusUnauthorized, "INVALID_TICKET", "missing websocket ticket")
		return
	}

	userID, err := h.tickets.ConsumeTicket(r.Context(), ticket)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "INVALID_TICKET", "websocket ticket is invalid or expired")
		return
	}

	guildList, err := h.guilds.ListByUser(r.Context(), userID)
	if err != nil {
		h.log.Error("failed to list user guilds for ws handshake", "user_id", userID, "err", err)
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load user guilds")
		return
	}

	guildIDs := make([]uuid.UUID, 0, len(guildList))
	for _, g := range guildList {
		guildIDs = append(guildIDs, g.ID)
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	// Clear ReadTimeout/WriteTimeout inherited from net/http before transferring the conn to long-lived pumps.
	if underlying := conn.UnderlyingConn(); underlying != nil {
		_ = underlying.SetDeadline(time.Time{})
	}

	client := NewClient(h.hub, userID, guildIDs, conn, h.clientOpts)
	if !h.hub.Register(client) {
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(CloseGoingAway, "server shutting down"),
			time.Now().Add(time.Second),
		)
		_ = conn.Close()
	}
}

func (h *Handler) isOriginAllowed(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		// Allow non-browser / CLI / loadtest WebSocket clients without an Origin header.
		return true
	}
	_, ok := h.allowedOrigins[origin]
	return ok
}

type wsErrorEnvelope struct {
	Error ErrorPayload `json:"error"`
}

func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(wsErrorEnvelope{
		Error: ErrorPayload{
			Code:    code,
			Message: message,
		},
	})
}
