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

type broadcastAction uint8

const (
	broadcastOnly broadcastAction = iota
	broadcastThenRemoveGuild
	unsubscribeThenBroadcast
	subscribeThenBroadcast
)

type broadcastMsg struct {
	guildID uuid.UUID
	userID  uuid.UUID
	frame   []byte
	action  broadcastAction
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
	hookCh            chan func()
	done              chan struct{}
	pumpWG            sync.WaitGroup
	hookWG            sync.WaitGroup
	heartbeatInterval time.Duration
	log               *slog.Logger
	onUserOnline      func(userID uuid.UUID, guildIDs []uuid.UUID)
	onUserOffline     func(userID uuid.UUID, guildIDs []uuid.UUID)
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
		hookCh:            make(chan func(), 512),
		done:              make(chan struct{}),
		heartbeatInterval: heartbeatInterval,
		log:               log,
	}
}

// SetOnlineHook registers a callback invoked asynchronously when a user opens their first active session.
func (h *Hub) SetOnlineHook(fn func(userID uuid.UUID, guildIDs []uuid.UUID)) {
	h.submitCmd(context.Background(), func() {
		h.onUserOnline = fn
	})
}

// SetOfflineHook registers a callback invoked asynchronously when a user's last active session closes.
func (h *Hub) SetOfflineHook(fn func(userID uuid.UUID, guildIDs []uuid.UUID)) {
	h.submitCmd(context.Background(), func() {
		h.onUserOffline = fn
	})
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
	h.hookWG.Add(1)
	go func() {
		defer h.hookWG.Done()
		for fn := range h.hookCh {
			if fn != nil {
				fn()
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			close(h.done)
			for c := range h.clients {
				h.removeClient(c, CloseGoingAway, "server shutting down", false)
			}
			h.pumpWG.Wait()
			close(h.hookCh)
			h.hookWG.Wait()
			return

		case req := <-h.registerCh:
			ok := h.addClient(req.client)
			req.reply <- ok

		case c := <-h.unregisterCh:
			h.removeClient(c, CloseNormalClosure, "", true)

		case msg := <-h.broadcastCh:
			switch msg.action {
			case broadcastThenRemoveGuild:
				h.fanoutGuild(msg.guildID, msg.frame)
				h.removeGuildLocked(msg.guildID)
			case unsubscribeThenBroadcast:
				h.unsubscribeUserFromGuild(msg.userID, msg.guildID)
				h.fanoutGuild(msg.guildID, msg.frame)
			case subscribeThenBroadcast:
				h.subscribeUserToGuild(msg.userID, msg.guildID)
				h.fanoutGuild(msg.guildID, msg.frame)
			default:
				h.fanoutGuild(msg.guildID, msg.frame)
			}

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
	h.submitCmdSync(ctx, func() {
		h.subscribeUserToGuild(userID, guildID)
	})
}

// Unsubscribe dynamically removes all active sessions of userID from guildID's broadcast set (ADR-007).
func (h *Hub) Unsubscribe(ctx context.Context, userID, guildID uuid.UUID) {
	h.submitCmdSync(ctx, func() {
		h.unsubscribeUserFromGuild(userID, guildID)
	})
}

// RemoveGuild cleans up all client subscriptions when a guild is deleted.
func (h *Hub) RemoveGuild(ctx context.Context, guildID uuid.UUID) {
	h.submitCmdSync(ctx, func() {
		h.removeGuildLocked(guildID)
	})
}

// BroadcastToGuild enqueues frame for every client subscribed to guildID.
func (h *Hub) BroadcastToGuild(ctx context.Context, guildID uuid.UUID, frame []byte) bool {
	return h.enqueueBroadcast(ctx, broadcastMsg{
		guildID: guildID,
		frame:   frame,
		action:  broadcastOnly,
	})
}

// BroadcastAndRemoveGuild atomically fans out frame to guildID members and removes guildID subscriptions.
func (h *Hub) BroadcastAndRemoveGuild(ctx context.Context, guildID uuid.UUID, frame []byte) bool {
	return h.enqueueBroadcast(ctx, broadcastMsg{
		guildID: guildID,
		frame:   frame,
		action:  broadcastThenRemoveGuild,
	})
}

// UnsubscribeAndBroadcast atomically unsubscribes userID from guildID and broadcasts frame to remaining members.
func (h *Hub) UnsubscribeAndBroadcast(ctx context.Context, userID, guildID uuid.UUID, frame []byte) bool {
	return h.enqueueBroadcast(ctx, broadcastMsg{
		guildID: guildID,
		userID:  userID,
		frame:   frame,
		action:  unsubscribeThenBroadcast,
	})
}

// SubscribeAndBroadcast atomically subscribes active sessions of userID to guildID and broadcasts frame.
func (h *Hub) SubscribeAndBroadcast(ctx context.Context, userID, guildID uuid.UUID, frame []byte) bool {
	return h.enqueueBroadcast(ctx, broadcastMsg{
		guildID: guildID,
		userID:  userID,
		frame:   frame,
		action:  subscribeThenBroadcast,
	})
}

func (h *Hub) enqueueBroadcast(ctx context.Context, msg broadcastMsg) bool {
	if len(msg.frame) == 0 {
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case h.broadcastCh <- msg:
		return true
	case <-h.done:
		return false
	case <-ctx.Done():
		return false
	}
}

func (h *Hub) subscribeUserToGuild(userID, guildID uuid.UUID) {
	if userID == uuid.Nil || guildID == uuid.Nil {
		return
	}
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
}

func (h *Hub) unsubscribeUserFromGuild(userID, guildID uuid.UUID) {
	if userID == uuid.Nil || guildID == uuid.Nil {
		return
	}
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
}

func (h *Hub) removeGuildLocked(guildID uuid.UUID) {
	if guildID == uuid.Nil {
		return
	}
	guildSet := h.guilds[guildID]
	for c := range guildSet {
		delete(c.guilds, guildID)
	}
	delete(h.guilds, guildID)
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
			fn := h.onHeartbeat
			uid := c.userID
			h.dispatchHook(func() { fn(uid) })
		}
		h.enqueue(c, frame)
	})
}

// HandleTypingStart forwards a client's TYPING_START payload to the optional typing hook via the async hook worker.
func (h *Hub) HandleTypingStart(c *Client, data json.RawMessage) {
	if c == nil {
		return
	}
	payloadCopy := append(json.RawMessage(nil), data...)
	h.submitCmd(context.Background(), func() {
		if _, ok := h.clients[c]; !ok {
			return
		}
		if h.onTypingStart != nil {
			fn := h.onTypingStart
			h.dispatchHook(func() { fn(c, payloadCopy) })
		}
	})
}

func (h *Hub) dispatchHook(fn func()) {
	select {
	case <-h.done:
		return
	default:
	}
	select {
	case h.hookCh <- fn:
	default:
		h.log.Warn("dropping hub hook callback: queue full")
	}
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
	wasOffline := !ok || len(userSet) == 0
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

	if wasOffline && h.onUserOnline != nil {
		fn := h.onUserOnline
		uid := c.userID
		guildsCopy := append([]uuid.UUID(nil), guildList...)
		h.dispatchHook(func() { fn(uid, guildsCopy) })
	}

	return true
}

func (h *Hub) removeClient(c *Client, code int, reason string, emitOffline bool) {
	if _, ok := h.clients[c]; !ok {
		return
	}

	delete(h.clients, c)

	guildList := make([]uuid.UUID, 0, len(c.guilds))
	for gid := range c.guilds {
		guildList = append(guildList, gid)
		if guildSet, ok := h.guilds[gid]; ok {
			delete(guildSet, c)
			if len(guildSet) == 0 {
				delete(h.guilds, gid)
			}
		}
		delete(c.guilds, gid)
	}

	lastSessionClosed := false
	if userSet, ok := h.users[c.userID]; ok {
		delete(userSet, c)
		if len(userSet) == 0 {
			delete(h.users, c.userID)
			lastSessionClosed = true
		}
	}

	c.signalStop(code, reason)
	close(c.send)

	if emitOffline && lastSessionClosed && h.onUserOffline != nil {
		fn := h.onUserOffline
		uid := c.userID
		h.dispatchHook(func() { fn(uid, guildList) })
	}
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
		h.removeClient(c, CloseSlowConsumer, "slow consumer", true)
	}
}
