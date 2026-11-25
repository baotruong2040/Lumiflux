package gateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/truong/discord-clone-api/internal/domain"
)

const defaultHeartbeatInterval = 25 * time.Second

// HubStats provides a snapshot of active connections and map cardinalities for diagnostics/tests.
type HubStats struct {
	Clients int
	Users   int
	Guilds  int
}

type registerReq struct {
	client *Client
	reply  chan bool
}

type broadcastMsg struct {
	guildID uuid.UUID
	frame   []byte
}

type clientFrame struct {
	client *Client
	frame  []byte
}

// Hub implements a single-goroutine Actor managing WebSocket clients, user sessions, and guild subscriptions.
// Implements domain.GuildSubscriptionManager (ADR-007).
type Hub struct {
	clients           map[*Client]struct{}
	users             map[uuid.UUID]map[*Client]struct{}
	guilds            map[uuid.UUID]map[*Client]struct{}
	registerCh        chan registerReq
	unregisterCh      chan *Client
	broadcastCh       chan broadcastMsg
	directCh          chan clientFrame
	cmdCh             chan func()
	done              chan struct{}
	pumpWG            sync.WaitGroup
	heartbeatInterval time.Duration
	log               *slog.Logger
	onHeartbeat       func(userID uuid.UUID)
	onTypingStart     func(client *Client, data json.RawMessage)
}

var _ domain.GuildSubscriptionManager = (*Hub)(nil)

// NewHub constructs an unstarted Hub actor. Call Run(ctx) in a dedicated goroutine before registering clients.
func NewHub(heartbeatInterval time.Duration, log *slog.Logger) *Hub {
	if heartbeatInterval <= 0 {
		heartbeatInterval = defaultHeartbeatInterval
	}
	if log == nil {
		log = slog.Default()
	}
	return &Hub{
		clients:           make(map[*Client]struct{}),
		users:             make(map[uuid.UUID]map[*Client]struct{}),
		guilds:            make(map[uuid.UUID]map[*Client]struct{}),
		registerCh:        make(chan registerReq, 64),
		unregisterCh:      make(chan *Client, 64),
		broadcastCh:       make(chan broadcastMsg, 256),
		directCh:          make(chan clientFrame, 128),
		cmdCh:             make(chan func(), 64),
		done:              make(chan struct{}),
		heartbeatInterval: heartbeatInterval,
		log:               log,
	}
}

// SetHeartbeatHook registers an optional callback invoked when a client sends HEARTBEAT.
func (h *Hub) SetHeartbeatHook(fn func(userID uuid.UUID)) {
	h.submitCmd(context.Background(), func() {
		h.onHeartbeat = fn
	})
}

// SetTypingHook registers an optional callback invoked when a client sends TYPING_START.
func (h *Hub) SetTypingHook(fn func(client *Client, data json.RawMessage)) {
	h.submitCmd(context.Background(), func() {
		h.onTypingStart = fn
	})
}

// Run owns and processes all state mutations on the hub actor goroutine until ctx is cancelled.
func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			close(h.done)
			for c := range h.clients {
				h.removeClient(c, CloseGoingAway, "server shutting down")
			}
			h.pumpWG.Wait()
			return

		case req := <-h.registerCh:
			ok := h.addClient(req.client)
			req.reply <- ok

		case c := <-h.unregisterCh:
			h.removeClient(c, CloseNormalClosure, "")

		case msg := <-h.broadcastCh:
			h.fanoutGuild(msg.guildID, msg.frame)

		case direct := <-h.directCh:
			if _, ok := h.clients[direct.client]; ok {
				h.enqueue(direct.client, direct.frame)
			}

		case fn := <-h.cmdCh:
			fn()
		}
	}
}

// Register adds client to the hub, launches its socket pumps if backed by a connection, and dispatches READY.
func (h *Hub) Register(client *Client) bool {
	if client == nil {
		return false
	}
	req := registerReq{
		client: client,
		reply:  make(chan bool, 1),
	}
	select {
	case h.registerCh <- req:
	case <-h.done:
		return false
	}
	select {
	case ok := <-req.reply:
		return ok
	case <-h.done:
		return false
	}
}

// Unregister removes client from all hub maps and stops its pumps. Safe and idempotent if called repeatedly.
func (h *Hub) Unregister(client *Client) {
	if client == nil {
		return
	}
	select {
	case h.unregisterCh <- client:
	case <-h.done:
	}
}

// Subscribe dynamically adds all active sessions of userID to guildID's broadcast set (ADR-007).
func (h *Hub) Subscribe(ctx context.Context, userID, guildID uuid.UUID) {
	h.submitCmd(ctx, func() {
		conns := h.users[userID]
		if len(conns) == 0 {
			return
		}
		guildSet, ok := h.guilds[guildID]
		if !ok {
			guildSet = make(map[*Client]struct{}, len(conns))
			h.guilds[guildID] = guildSet
		}
		for c := range conns {
			guildSet[c] = struct{}{}
			c.guilds[guildID] = struct{}{}
		}
	})
}

// Unsubscribe dynamically removes all active sessions of userID from guildID's broadcast set (ADR-007).
func (h *Hub) Unsubscribe(ctx context.Context, userID, guildID uuid.UUID) {
	h.submitCmd(ctx, func() {
		conns := h.users[userID]
		if len(conns) == 0 {
			return
		}
		guildSet := h.guilds[guildID]
		for c := range conns {
			delete(c.guilds, guildID)
			if guildSet != nil {
				delete(guildSet, c)
			}
		}
		if guildSet != nil && len(guildSet) == 0 {
			delete(h.guilds, guildID)
		}
	})
}

// RemoveGuild cleans up all client subscriptions when a guild is deleted.
func (h *Hub) RemoveGuild(ctx context.Context, guildID uuid.UUID) {
	h.submitCmd(ctx, func() {
		guildSet := h.guilds[guildID]
		for c := range guildSet {
			delete(c.guilds, guildID)
		}
		delete(h.guilds, guildID)
	})
}

// BroadcastToGuild enqueues frame for every client subscribed to guildID.
func (h *Hub) BroadcastToGuild(ctx context.Context, guildID uuid.UUID, frame []byte) bool {
	if len(frame) == 0 {
		return false
	}
	select {
	case h.broadcastCh <- broadcastMsg{guildID: guildID, frame: frame}:
		return true
	case <-h.done:
		return false
	case <-ctx.Done():
		return false
	}
}

// SendToClient enqueues a frame for a single connected client.
func (h *Hub) SendToClient(c *Client, frame []byte) bool {
	if c == nil || len(frame) == 0 {
		return false
	}
	select {
	case h.directCh <- clientFrame{client: c, frame: frame}:
		return true
	case <-h.done:
		return false
	}
}

// SendError dispatches a protocol ERROR frame to client c.
func (h *Hub) SendError(c *Client, code, message string) {
	frame, err := MarshalFrame(EventError, ErrorPayload{
		Code:    code,
		Message: message,
	})
	if err != nil {
		return
	}
	h.SendToClient(c, frame)
}

// HandleHeartbeat responds with HEARTBEAT_ACK and invokes the optional heartbeat hook on the actor goroutine.
func (h *Hub) HandleHeartbeat(c *Client) {
	if c == nil {
		return
	}
	frame, err := MarshalFrame(EventHeartbeatAck, struct{}{})
	if err != nil {
		return
	}
	h.submitCmd(context.Background(), func() {
		if _, ok := h.clients[c]; !ok {
			return
		}
		if h.onHeartbeat != nil {
			h.onHeartbeat(c.userID)
		}
		h.enqueue(c, frame)
	})
}

// HandleTypingStart forwards a client's TYPING_START payload to the optional typing hook on the actor goroutine.
func (h *Hub) HandleTypingStart(c *Client, data json.RawMessage) {
	if c == nil {
		return
	}
	h.submitCmd(context.Background(), func() {
		if _, ok := h.clients[c]; !ok {
			return
		}
		if h.onTypingStart != nil {
			h.onTypingStart(c, data)
		}
	})
}

// Stats queries current hub map sizes safely through the actor goroutine.
func (h *Hub) Stats() HubStats {
	reply := make(chan HubStats, 1)
	ok := h.submitCmdSync(context.Background(), func() {
		reply <- HubStats{
			Clients: len(h.clients),
			Users:   len(h.users),
			Guilds:  len(h.guilds),
		}
	})
	if !ok {
		return HubStats{}
	}
	return <-reply
}

// Done returns a channel closed once Hub.Run initiates shutdown.
func (h *Hub) Done() <-chan struct{} {
	return h.done
}

func (h *Hub) submitCmd(ctx context.Context, fn func()) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case h.cmdCh <- fn:
	case <-h.done:
	case <-ctx.Done():
	}
}

func (h *Hub) submitCmdSync(ctx context.Context, fn func()) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	done := make(chan struct{})
	wrapped := func() {
		defer close(done)
		fn()
	}
	select {
	case h.cmdCh <- wrapped:
	case <-h.done:
		return false
	case <-ctx.Done():
		return false
	}
	select {
	case <-done:
		return true
	case <-h.done:
		return false
	case <-ctx.Done():
		return false
	}
}

func (h *Hub) addClient(c *Client) bool {
	h.clients[c] = struct{}{}

	userSet, ok := h.users[c.userID]
	if !ok {
		userSet = make(map[*Client]struct{})
		h.users[c.userID] = userSet
	}
	userSet[c] = struct{}{}

	seenGuilds := make(map[uuid.UUID]struct{}, len(c.initialGuilds))
	guildList := make([]uuid.UUID, 0, len(c.initialGuilds))
	for _, gid := range c.initialGuilds {
		if gid == uuid.Nil {
			continue
		}
		if _, dup := seenGuilds[gid]; dup {
			continue
		}
		seenGuilds[gid] = struct{}{}
		guildList = append(guildList, gid)
		c.guilds[gid] = struct{}{}

		guildSet, exists := h.guilds[gid]
		if !exists {
			guildSet = make(map[*Client]struct{})
			h.guilds[gid] = guildSet
		}
		guildSet[c] = struct{}{}
	}

	// Register socket pumps inside the actor goroutine before Wait() can ever execute on shutdown.
	if c.conn != nil {
		h.pumpWG.Add(2)
		go c.writePump()
		go c.readPump()
	}

	readyFrame, err := MarshalFrame(EventReady, ReadyPayload{
		UserID:              c.userID,
		GuildIDs:            guildList,
		HeartbeatIntervalMs: h.heartbeatInterval.Milliseconds(),
	})
	if err == nil {
		h.enqueue(c, readyFrame)
	}

	return true
}

func (h *Hub) removeClient(c *Client, code int, reason string) {
	if _, ok := h.clients[c]; !ok {
		return
	}

	delete(h.clients, c)

	if userSet, ok := h.users[c.userID]; ok {
		delete(userSet, c)
		if len(userSet) == 0 {
			delete(h.users, c.userID)
		}
	}

	for gid := range c.guilds {
		if guildSet, ok := h.guilds[gid]; ok {
			delete(guildSet, c)
			if len(guildSet) == 0 {
				delete(h.guilds, gid)
			}
		}
		delete(c.guilds, gid)
	}

	c.signalStop(code, reason)
	close(c.send)
}

func (h *Hub) fanoutGuild(guildID uuid.UUID, frame []byte) {
	guildSet := h.guilds[guildID]
	for c := range guildSet {
		h.enqueue(c, frame)
	}
}

func (h *Hub) enqueue(c *Client, frame []byte) {
	select {
	case c.send <- frame:
	default:
		h.log.Warn("disconnecting slow websocket consumer", "user_id", c.userID)
		h.removeClient(c, CloseSlowConsumer, "slow consumer")
	}
}
