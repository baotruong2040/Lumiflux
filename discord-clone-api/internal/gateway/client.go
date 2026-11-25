package gateway

import (
	"encoding/json"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	defaultMaxFrameBytes int64 = 8192
	defaultWriteWait           = 10 * time.Second
	defaultPongWait            = 60 * time.Second
)

// ClientOptions tunes read/write timeouts and limits for a WebSocket client connection.
type ClientOptions struct {
	MaxFrameBytes int64
	WriteWait     time.Duration
	PongWait      time.Duration
	Logger        *slog.Logger
}

type closeInfo struct {
	code   int
	reason string
}

// Client represents a single WebSocket session (or headless test consumer when conn is nil).
// Its guilds map and send channel writes are owned exclusively by Hub.Run.
type Client struct {
	hub           *Hub
	userID        uuid.UUID
	conn          *websocket.Conn
	send          chan []byte
	initialGuilds []uuid.UUID
	guilds        map[uuid.UUID]struct{}
	stopCh        chan struct{}
	stopOnce      sync.Once
	closeInfo     atomic.Pointer[closeInfo]
	log           *slog.Logger
	maxFrameBytes int64
	writeWait     time.Duration
	pongWait      time.Duration
}

// NewClient constructs a WebSocket-backed Client for use with Hub.Register.
func NewClient(
	hub *Hub,
	userID uuid.UUID,
	initialGuildIDs []uuid.UUID,
	conn *websocket.Conn,
	opts ClientOptions,
) *Client {
	maxBytes := opts.MaxFrameBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxFrameBytes
	}
	writeWait := opts.WriteWait
	if writeWait <= 0 {
		writeWait = defaultWriteWait
	}
	pongWait := opts.PongWait
	if pongWait <= 0 {
		pongWait = defaultPongWait
	}
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}

	guildCopy := make([]uuid.UUID, len(initialGuildIDs))
	copy(guildCopy, initialGuildIDs)

	return &Client{
		hub:           hub,
		userID:        userID,
		conn:          conn,
		send:          make(chan []byte, SendBufferSize),
		initialGuilds: guildCopy,
		guilds:        make(map[uuid.UUID]struct{}, len(guildCopy)),
		stopCh:        make(chan struct{}),
		log:           log,
		maxFrameBytes: maxBytes,
		writeWait:     writeWait,
		pongWait:      pongWait,
	}
}

// NewFakeClient constructs a socketless Client whose Send() channel can be drained or left full in tests/benchmarks.
func NewFakeClient(hub *Hub, userID uuid.UUID, initialGuildIDs []uuid.UUID) *Client {
	return NewClient(hub, userID, initialGuildIDs, nil, ClientOptions{})
}

// UserID returns the authenticated user identifier owning this client session.
func (c *Client) UserID() uuid.UUID {
	return c.userID
}

// Send returns the read-only outbound frame channel for test and benchmark consumers.
func (c *Client) Send() <-chan []byte {
	return c.send
}

// Done returns a channel closed when the hub evicts or unregisters this client.
func (c *Client) Done() <-chan struct{} {
	return c.stopCh
}

// CloseCode returns the WebSocket close code recorded by Hub on eviction, or 0 if not closed.
func (c *Client) CloseCode() int {
	info := c.closeInfo.Load()
	if info == nil {
		return 0
	}
	return info.code
}

func (c *Client) signalStop(code int, reason string) {
	if code > 0 {
		c.closeInfo.CompareAndSwap(nil, &closeInfo{code: code, reason: reason})
	}
	c.stopOnce.Do(func() {
		close(c.stopCh)
	})
}

func (c *Client) readPump() {
	defer func() {
		c.hub.Unregister(c)
		if c.conn != nil {
			_ = c.conn.Close()
		}
		c.hub.pumpWG.Done()
	}()

	c.conn.SetReadLimit(c.maxFrameBytes)
	_ = c.conn.SetReadDeadline(time.Now().Add(c.pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(c.pongWait))
	})

	for {
		_, payload, err := c.conn.ReadMessage()
		if err != nil {
			return
		}

		var frame InboundFrame
		if unmarshalErr := json.Unmarshal(payload, &frame); unmarshalErr != nil {
			c.hub.SendError(c, "INVALID_FRAME", "invalid JSON frame")
			continue
		}

		switch frame.T {
		case EventHeartbeat:
			_ = c.conn.SetReadDeadline(time.Now().Add(c.pongWait))
			c.hub.HandleHeartbeat(c)
		case EventTypingStart:
			c.hub.HandleTypingStart(c, frame.D)
		default:
			c.hub.SendError(c, "UNKNOWN_EVENT", "unsupported websocket event type")
		}
	}
}

func (c *Client) writePump() {
	defer func() {
		if c.conn != nil {
			_ = c.conn.Close()
		}
		c.hub.pumpWG.Done()
	}()

	for {
		// Prioritize eviction/shutdown close signals over draining buffered frames (TC-HUB-01, TC-HUB-04).
		select {
		case <-c.stopCh:
			c.writeCloseFrame()
			return
		default:
		}

		select {
		case <-c.stopCh:
			c.writeCloseFrame()
			return
		case frame, ok := <-c.send:
			if !ok {
				c.writeCloseFrame()
				return
			}
			_ = c.conn.SetWriteDeadline(time.Now().Add(c.writeWait))
			if err := c.conn.WriteMessage(websocket.TextMessage, frame); err != nil {
				return
			}
		}
	}
}

func (c *Client) writeCloseFrame() {
	if c.conn == nil {
		return
	}
	info := c.closeInfo.Load()
	code := CloseNormalClosure
	reason := ""
	if info != nil && info.code > 0 {
		code = info.code
		reason = info.reason
	}
	_ = c.conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(code, reason),
		time.Now().Add(time.Second),
	)
}
