package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime/pprof"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/truong/discord-clone-api/internal/domain"
	"github.com/truong/discord-clone-api/pkg/idgen"
	"github.com/truong/discord-clone-api/pkg/logger"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

type memoryTicketConsumer struct {
	mu      sync.Mutex
	tickets map[string]uuid.UUID
}

func newMemoryTicketConsumer() *memoryTicketConsumer {
	return &memoryTicketConsumer{
		tickets: make(map[string]uuid.UUID),
	}
}

func (m *memoryTicketConsumer) Add(ticket string, userID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tickets[ticket] = userID
}

func (m *memoryTicketConsumer) ConsumeTicket(_ context.Context, ticket string) (uuid.UUID, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	uid, ok := m.tickets[ticket]
	if !ok {
		return uuid.Nil, domain.NewError(domain.ErrInvalidToken, "INVALID_TICKET", "invalid ticket")
	}
	delete(m.tickets, ticket)
	return uid, nil
}

type memoryGuildLister struct {
	mu     sync.RWMutex
	guilds map[uuid.UUID][]domain.Guild
}

func newMemoryGuildLister() *memoryGuildLister {
	return &memoryGuildLister{
		guilds: make(map[uuid.UUID][]domain.Guild),
	}
}

func (m *memoryGuildLister) SetGuilds(userID uuid.UUID, list []domain.Guild) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.guilds[userID] = list
}

func (m *memoryGuildLister) ListByUser(_ context.Context, userID uuid.UUID) ([]domain.Guild, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]domain.Guild(nil), m.guilds[userID]...), nil
}

func startTestGatewayServer(t *testing.T, allowedOrigins []string) (*Hub, *memoryTicketConsumer, *memoryGuildLister, *httptest.Server, context.CancelFunc) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	log := logger.NewWithWriter(io.Discard, "error")
	hub := NewHub(25*time.Second, log)
	hubDone := make(chan struct{})
	go func() {
		defer close(hubDone)
		hub.Run(ctx)
	}()

	tickets := newMemoryTicketConsumer()
	guilds := newMemoryGuildLister()
	handler := NewHandler(hub, tickets, guilds, allowedOrigins, log)
	ts := httptest.NewServer(handler)

	t.Cleanup(func() {
		cancel()
		<-hubDone
		ts.Close()
	})

	return hub, tickets, guilds, ts, cancel
}

func wsURL(httpURL, ticket string) string {
	u := "ws" + strings.TrimPrefix(httpURL, "http") + "/ws"
	if ticket != "" {
		u += "?ticket=" + ticket
	}
	return u
}

func dialTestWS(t *testing.T, serverURL, ticket, origin string) *websocket.Conn {
	t.Helper()
	hdr := http.Header{}
	if origin != "" {
		hdr.Set("Origin", origin)
	}
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL(serverURL, ticket), hdr)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err)
	return conn
}

func readFrame(t *testing.T, conn *websocket.Conn) InboundFrame {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, payload, err := conn.ReadMessage()
	require.NoError(t, err)
	var frame InboundFrame
	require.NoError(t, json.Unmarshal(payload, &frame))
	return frame
}

func TestHandler_TicketAuthAndOriginCheck(t *testing.T) {
	t.Cleanup(func() { goleak.VerifyNone(t) })

	allowed := []string{
		"http://localhost:5173",
		"http://127.0.0.1:5173",
		"tauri://localhost",
		"http://tauri.localhost",
		"https://tauri.localhost",
	}
	_, tickets, guilds, ts, _ := startTestGatewayServer(t, allowed)

	userID, _ := idgen.NewUUIDv7()
	guildID, _ := idgen.NewUUIDv7()
	guilds.SetGuilds(userID, []domain.Guild{{ID: guildID, Name: "Gateway Guild", OwnerID: userID}})

	// Missing ticket -> 401
	_, resp, err := websocket.DefaultDialer.Dial(wsURL(ts.URL, ""), nil)
	require.Error(t, err)
	require.NotNil(t, resp)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// Disallowed origin -> 403
	tickets.Add("ticket-origin-bad", userID)
	badHdr := http.Header{"Origin": {"https://evil.example.com"}}
	_, resp, err = websocket.DefaultDialer.Dial(wsURL(ts.URL, "ticket-origin-bad"), badHdr)
	require.Error(t, err)
	require.NotNil(t, resp)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	// Allowed Web & Tauri origins succeed, and ticket replay -> 401
	for i, origin := range allowed {
		ticket := fmt.Sprintf("ticket-ok-%d", i)
		tickets.Add(ticket, userID)
		conn := dialTestWS(t, ts.URL, ticket, origin)

		readyFrame := readFrame(t, conn)
		assert.Equal(t, EventReady, readyFrame.T)

		require.NoError(t, conn.Close())

		// Reusing same ticket -> 401
		_, replayResp, replayErr := websocket.DefaultDialer.Dial(wsURL(ts.URL, ticket), http.Header{"Origin": {origin}})
		require.Error(t, replayErr)
		require.NotNil(t, replayResp)
		_ = replayResp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, replayResp.StatusCode)
	}
}

func TestHub_ReadyHeartbeatAndDynamicSubscriptions(t *testing.T) {
	t.Cleanup(func() { goleak.VerifyNone(t) })

	hub, tickets, guilds, ts, _ := startTestGatewayServer(t, []string{"http://localhost:5173"})
	ctx := context.Background()

	userID, _ := idgen.NewUUIDv7()
	guildA, _ := idgen.NewUUIDv7()
	guildB, _ := idgen.NewUUIDv7()
	guilds.SetGuilds(userID, []domain.Guild{{ID: guildA, Name: "Guild A", OwnerID: userID}})

	var heartbeatCount atomic.Int32
	hub.SetHeartbeatHook(func(uid uuid.UUID) {
		if uid == userID {
			heartbeatCount.Add(1)
		}
	})

	tickets.Add("ticket-main", userID)
	conn := dialTestWS(t, ts.URL, "ticket-main", "http://localhost:5173")
	defer func() { _ = conn.Close() }()

	// Verify READY frame payload
	readyFrame := readFrame(t, conn)
	assert.Equal(t, EventReady, readyFrame.T)
	var ready ReadyPayload
	require.NoError(t, json.Unmarshal(readyFrame.D, &ready))
	assert.Equal(t, userID, ready.UserID)
	assert.Equal(t, []uuid.UUID{guildA}, ready.GuildIDs)
	assert.Equal(t, int64(25000), ready.HeartbeatIntervalMs)

	// HEARTBEAT -> HEARTBEAT_ACK + hook invoked
	require.NoError(t, conn.WriteJSON(OutboundFrame{T: EventHeartbeat}))
	ackFrame := readFrame(t, conn)
	assert.Equal(t, EventHeartbeatAck, ackFrame.T)
	require.Eventually(t, func() bool { return heartbeatCount.Load() == 1 }, time.Second, 5*time.Millisecond)

	// Malformed JSON -> ERROR frame
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("{bad-json")))
	errFrame := readFrame(t, conn)
	assert.Equal(t, EventError, errFrame.T)

	// Subscribe to guildB -> broadcast reaches client; Unsubscribe -> stops reaching client
	hub.Subscribe(ctx, userID, guildB)
	msgFrame, err := MarshalFrame(EventMessageCreate, map[string]string{"id": "m1"})
	require.NoError(t, err)
	require.True(t, hub.BroadcastToGuild(ctx, guildB, msgFrame))

	received := readFrame(t, conn)
	assert.Equal(t, EventMessageCreate, received.T)

	hub.Unsubscribe(ctx, userID, guildB)
	msgFrame2, err := MarshalFrame(EventMessageCreate, map[string]string{"id": "m2"})
	require.NoError(t, err)
	require.True(t, hub.BroadcastToGuild(ctx, guildB, msgFrame2))

	// Guild A still reaches client; Guild B m2 should NOT arrive first
	msgFrameA, err := MarshalFrame(EventMessageCreate, map[string]string{"id": "a1"})
	require.NoError(t, err)
	require.True(t, hub.BroadcastToGuild(ctx, guildA, msgFrameA))

	next := readFrame(t, conn)
	assert.Contains(t, string(next.D), `"a1"`)
}

func TestHub_TC_HUB_01_SlowConsumer4002(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctx, cancel := context.WithCancel(context.Background())
	log := logger.NewWithWriter(io.Discard, "error")
	hub := NewHub(25*time.Second, log)
	hubDone := make(chan struct{})
	go func() {
		defer close(hubDone)
		hub.Run(ctx)
	}()
	defer func() {
		cancel()
		<-hubDone
	}()

	guildID, _ := idgen.NewUUIDv7()
	slowUser, _ := idgen.NewUUIDv7()
	fastUser, _ := idgen.NewUUIDv7()

	slowClient := NewFakeClient(hub, slowUser, []uuid.UUID{guildID})
	fastClient := NewFakeClient(hub, fastUser, []uuid.UUID{guildID})

	require.True(t, hub.Register(slowClient))
	require.True(t, hub.Register(fastClient))

	// Fast client actively drains its send channel; slow client reads nothing.
	var fastReceived atomic.Int32
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range fastClient.Send() {
			fastReceived.Add(1)
		}
	}()

	// READY already uses 1 slot out of 256, so 256 broadcasts overflow on slot 257.
	payload, err := MarshalFrame(EventMessageCreate, map[string]string{"seq": "test"})
	require.NoError(t, err)
	for range SendBufferSize {
		require.True(t, hub.BroadcastToGuild(ctx, guildID, payload))
	}

	// Slow consumer must be disconnected with code 4002 while fast consumer receives READY + all 256 frames.
	select {
	case <-slowClient.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for slow consumer eviction")
	}
	assert.Equal(t, CloseSlowConsumer, slowClient.CloseCode())

	require.Eventually(t, func() bool {
		return fastReceived.Load() == int32(SendBufferSize+1) && hub.Stats().Clients == 1
	}, 2*time.Second, 10*time.Millisecond)

	hub.Unregister(fastClient)
	wg.Wait()
	assert.Equal(t, HubStats{Clients: 0, Users: 0, Guilds: 0}, hub.Stats())
}

func TestHub_TC_HUB_02_AbruptCloseMidFanout(t *testing.T) {
	t.Cleanup(func() { goleak.VerifyNone(t) })

	hub, tickets, guilds, ts, _ := startTestGatewayServer(t, nil)
	ctx := context.Background()
	guildID, _ := idgen.NewUUIDv7()

	const totalClients = 20
	conns := make([]*websocket.Conn, 0, totalClients)
	for i := range totalClients {
		uid, _ := idgen.NewUUIDv7()
		guilds.SetGuilds(uid, []domain.Guild{{ID: guildID, Name: "Fanout Guild", OwnerID: uid}})
		ticket := fmt.Sprintf("fanout-ticket-%d", i)
		tickets.Add(ticket, uid)
		c := dialTestWS(t, ts.URL, ticket, "")
		_ = readFrame(t, c) // consume READY
		conns = append(conns, c)
	}
	require.Equal(t, HubStats{Clients: totalClients, Users: totalClients, Guilds: 1}, hub.Stats())

	// Drain readers for surviving half of connections so they stay healthy.
	var readerWG sync.WaitGroup
	for i := totalClients / 2; i < totalClients; i++ {
		readerWG.Add(1)
		go func(c *websocket.Conn) {
			defer readerWG.Done()
			for {
				if _, _, err := c.ReadMessage(); err != nil {
					return
				}
			}
		}(conns[i])
	}

	// Abruptly close first half while concurrent goroutines fan out frames.
	var writerWG sync.WaitGroup
	frame, err := MarshalFrame(EventMessageCreate, map[string]string{"content": "burst"})
	require.NoError(t, err)

	for range 4 {
		writerWG.Add(1)
		go func() {
			defer writerWG.Done()
			for range 50 {
				hub.BroadcastToGuild(ctx, guildID, frame)
			}
		}()
	}

	for i := range totalClients / 2 {
		_ = conns[i].Close()
	}

	writerWG.Wait()

	// Remaining clients stay connected until closed; once all close, all 3 maps empty cleanly.
	for i := totalClients / 2; i < totalClients; i++ {
		_ = conns[i].Close()
	}
	readerWG.Wait()

	require.Eventually(t, func() bool {
		return hub.Stats() == (HubStats{Clients: 0, Users: 0, Guilds: 0})
	}, 3*time.Second, 15*time.Millisecond)
}

func TestHub_TC_HUB_03_DoubleUnregister(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctx, cancel := context.WithCancel(context.Background())
	hub := NewHub(25*time.Second, logger.NewWithWriter(io.Discard, "error"))
	hubDone := make(chan struct{})
	go func() {
		defer close(hubDone)
		hub.Run(ctx)
	}()
	defer func() {
		cancel()
		<-hubDone
	}()

	uid, _ := idgen.NewUUIDv7()
	gid, _ := idgen.NewUUIDv7()
	client := NewFakeClient(hub, uid, []uuid.UUID{gid})
	require.True(t, hub.Register(client))

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hub.Unregister(client)
		}()
	}
	wg.Wait()

	hub.Unregister(client)
	assert.Equal(t, HubStats{Clients: 0, Users: 0, Guilds: 0}, hub.Stats())
}

func TestHub_TC_HUB_04_GracefulShutdown1001(t *testing.T) {
	t.Cleanup(func() { goleak.VerifyNone(t) })

	hub, tickets, guilds, ts, cancel := startTestGatewayServer(t, nil)
	guildID, _ := idgen.NewUUIDv7()

	const nClients = 5
	conns := make([]*websocket.Conn, 0, nClients)
	for i := range nClients {
		uid, _ := idgen.NewUUIDv7()
		guilds.SetGuilds(uid, []domain.Guild{{ID: guildID, Name: "Shutdown Guild", OwnerID: uid}})
		ticket := fmt.Sprintf("shutdown-ticket-%d", i)
		tickets.Add(ticket, uid)
		c := dialTestWS(t, ts.URL, ticket, "")
		_ = readFrame(t, c) // READY
		conns = append(conns, c)
	}
	assert.Equal(t, nClients, hub.Stats().Clients)

	// Cancel Hub context -> Hub closes every client with code 1001 and Run exits cleanly.
	cancel()
	select {
	case <-hub.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("hub.Done() did not close after ctx cancel")
	}

	for _, c := range conns {
		_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, _, err := c.ReadMessage()
		require.Error(t, err)
		assert.True(t, websocket.IsCloseError(err, CloseGoingAway), "expected WebSocket 1001 GoingAway, got %v", err)
		_ = c.Close()
	}
}

func TestHub_TC_HUB_05_GoroutineBaselinePprof(t *testing.T) {
	defer goleak.VerifyNone(t)

	gp := pprof.Lookup("goroutine")
	require.NotNil(t, gp)
	baseline := gp.Count()

	ctx, cancel := context.WithCancel(context.Background())
	hub := NewHub(25*time.Second, logger.NewWithWriter(io.Discard, "error"))
	hubDone := make(chan struct{})
	go func() {
		defer close(hubDone)
		hub.Run(ctx)
	}()

	guildID, _ := idgen.NewUUIDv7()
	const clientCount = 1000
	clients := make([]*Client, 0, clientCount)
	var drainWG sync.WaitGroup

	for range clientCount {
		uid, _ := idgen.NewUUIDv7()
		c := NewFakeClient(hub, uid, []uuid.UUID{guildID})
		require.True(t, hub.Register(c))
		clients = append(clients, c)

		drainWG.Add(1)
		go func(cl *Client) {
			defer drainWG.Done()
			for range cl.Send() {
			}
		}(c)
	}

	frame, err := MarshalFrame(EventMessageCreate, map[string]string{"content": "pprof test"})
	require.NoError(t, err)
	require.True(t, hub.BroadcastToGuild(ctx, guildID, frame))

	for _, c := range clients {
		hub.Unregister(c)
	}
	drainWG.Wait()
	assert.Equal(t, HubStats{}, hub.Stats())

	cancel()
	<-hubDone

	require.Eventually(t, func() bool {
		return gp.Count() <= baseline+2
	}, 2*time.Second, 15*time.Millisecond, "goroutines should return near baseline (%d)", baseline)
}

func TestHub_MultiTabPresenceHooks(t *testing.T) {
	t.Cleanup(func() { goleak.VerifyNone(t) })

	ctx, cancel := context.WithCancel(context.Background())
	log := logger.NewWithWriter(io.Discard, "error")
	hub := NewHub(25*time.Second, log)
	hubDone := make(chan struct{})
	go func() {
		defer close(hubDone)
		hub.Run(ctx)
	}()
	defer func() {
		cancel()
		<-hubDone
	}()

	var onlineEvents atomic.Int32
	var offlineEvents atomic.Int32
	var mu sync.Mutex
	var lastOfflineGuilds []uuid.UUID

	hub.SetOnlineHook(func(_ uuid.UUID, _ []uuid.UUID) {
		onlineEvents.Add(1)
	})
	hub.SetOfflineHook(func(_ uuid.UUID, guildIDs []uuid.UUID) {
		mu.Lock()
		lastOfflineGuilds = append([]uuid.UUID(nil), guildIDs...)
		mu.Unlock()
		offlineEvents.Add(1)
	})

	userID := idgen.MustUUIDv7()
	guild1 := idgen.MustUUIDv7()
	guild2 := idgen.MustUUIDv7()

	tab1 := NewFakeClient(hub, userID, []uuid.UUID{guild1})
	tab2 := NewFakeClient(hub, userID, []uuid.UUID{guild1})

	// 0 -> 1 triggers online once
	require.True(t, hub.Register(tab1))
	require.Eventually(t, func() bool { return onlineEvents.Load() == 1 }, time.Second, 5*time.Millisecond)

	// 1 -> 2 does NOT trigger another online event
	require.True(t, hub.Register(tab2))
	time.Sleep(30 * time.Millisecond)
	assert.Equal(t, int32(1), onlineEvents.Load())

	// Subscribe adds guild2 to both tabs
	hub.Subscribe(ctx, userID, guild2)

	// 2 -> 1 does NOT emit offline
	hub.Unregister(tab1)
	time.Sleep(30 * time.Millisecond)
	assert.Equal(t, int32(0), offlineEvents.Load())

	// 1 -> 0 emits offline once with all subscribed guilds
	hub.Unregister(tab2)
	require.Eventually(t, func() bool { return offlineEvents.Load() == 1 }, time.Second, 5*time.Millisecond)
	mu.Lock()
	assert.ElementsMatch(t, []uuid.UUID{guild1, guild2}, lastOfflineGuilds)
	mu.Unlock()
}

func BenchmarkFanout(b *testing.B) {
	frame, err := MarshalFrame(EventMessageCreate, map[string]any{
		"id":        "0196a1b2-3c4d-7e8f-9a0b-1c2d3e4f5a70",
		"channelId": "0196a1b2-3c4d-7e8f-9a0b-1c2d3e4f5a6d",
		"content":   bytes.Repeat([]byte("a"), 128),
	})
	if err != nil {
		b.Fatal(err)
	}

	for _, clientCount := range []int{1000, 10000} {
		b.Run(fmt.Sprintf("clients_%d", clientCount), func(b *testing.B) {
			ctx, cancel := context.WithCancel(context.Background())
			hub := NewHub(25*time.Second, logger.NewWithWriter(io.Discard, "error"))
			hubDone := make(chan struct{})
			go func() {
				defer close(hubDone)
				hub.Run(ctx)
			}()

			guildID := uuid.Must(idgen.NewUUIDv7())
			var drainWG sync.WaitGroup
			for range clientCount {
				uid := uuid.Must(idgen.NewUUIDv7())
				c := NewFakeClient(hub, uid, []uuid.UUID{guildID})
				if !hub.Register(c) {
					b.Fatal("failed to register fake client")
				}
				drainWG.Add(1)
				go func(cl *Client) {
					defer drainWG.Done()
					for range cl.Send() {
					}
				}(c)
			}

			// Wait until Hub has registered all clients cleanly before starting benchmark timer.
			_ = hub.Stats()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				hub.BroadcastToGuild(ctx, guildID, frame)
				// Drain actor backlog periodically so channel capacity (256) does not overflow slow bench consumers.
				if i%128 == 127 {
					_ = hub.Stats()
				}
			}
			// Actor barrier: ensure every queued broadcast finished fanning out to all clients.
			_ = hub.Stats()
			b.StopTimer()

			cancel()
			<-hubDone
			drainWG.Wait()
		})
	}
}
