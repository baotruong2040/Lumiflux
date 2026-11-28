package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/truong/discord-clone-api/internal/controller/dto"
	"github.com/truong/discord-clone-api/internal/gateway"
)

func dialAuthenticatedWS(t *testing.T, ts *httptest.Server, accessToken string) (*websocket.Conn, gateway.ReadyPayload) {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/ws/ticket", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var ticketResp dto.WSTicketResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&ticketResp))
	require.NotEmpty(t, ticketResp.Ticket)

	wsTarget := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws?ticket=" + ticketResp.Ticket
	wsHdr := http.Header{"Origin": {"http://localhost:5173"}}
	conn, wsResp, err := websocket.DefaultDialer.Dial(wsTarget, wsHdr)
	if wsResp != nil {
		_ = wsResp.Body.Close()
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	frame := readWSFrame(t, conn, 3*time.Second)
	require.Equal(t, gateway.EventReady, frame.T)

	var ready gateway.ReadyPayload
	require.NoError(t, json.Unmarshal(frame.D, &ready))
	return conn, ready
}

func readWSFrame(t *testing.T, conn *websocket.Conn, timeout time.Duration) gateway.InboundFrame {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(timeout)))
	_, raw, err := conn.ReadMessage()
	require.NoError(t, err)

	var frame gateway.InboundFrame
	require.NoError(t, json.Unmarshal(raw, &frame))
	return frame
}

func readWSFrameOfType(t *testing.T, conn *websocket.Conn, wantType string, timeout time.Duration) gateway.InboundFrame {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		require.Greater(t, remaining, time.Duration(0), "timed out waiting for frame type %s", wantType)
		frame := readWSFrame(t, conn, remaining)
		if frame.T == wantType {
			return frame
		}
		if frame.T == gateway.EventPresenceUpdate {
			continue
		}
		require.Equal(t, wantType, frame.T, "unexpected websocket frame payload: %s", string(frame.D))
	}
}

func assertNoWSFrame(t *testing.T, conn *websocket.Conn, wait time.Duration) {
	t.Helper()
	deadline := time.Now().Add(wait)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return
		}
		require.NoError(t, conn.SetReadDeadline(deadline))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var frame gateway.InboundFrame
		if json.Unmarshal(raw, &frame) == nil && frame.T == gateway.EventPresenceUpdate {
			continue
		}
		t.Fatalf("expected no non-presence websocket frame, but received: %s", string(raw))
	}
}

func httpJSON(t *testing.T, method, url, accessToken string, body any, wantStatus int, out any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req, err := http.NewRequest(method, url, &buf)
	require.NoError(t, err)
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, wantStatus, resp.StatusCode)
	if out != nil {
		require.NoError(t, json.NewDecoder(resp.Body).Decode(out))
	}
}

func TestIntegration_TC_WS_01_SingleUseTicketAndUpgrade(t *testing.T) {
	env := setupIntegrationEnv(t)
	user := env.registerUser(t, "ws01_user")
	hdr := map[string]string{"Authorization": "Bearer " + user.AccessToken}

	guild, _ := createGuildWithGeneralChannel(t, env, hdr, "WS Integration Guild")

	// Issue single-use ticket via POST /api/v1/ws/ticket
	rec := env.do(t, http.MethodPost, "/api/v1/ws/ticket", nil, hdr, "")
	require.Equal(t, http.StatusOK, rec.Code)

	var ticketResp dto.WSTicketResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ticketResp))
	require.NotEmpty(t, ticketResp.Ticket)

	redisKey := "wsticket:" + ticketResp.Ticket
	env.trackRedisKey(redisKey)

	ttl, err := env.redis.TTL(context.Background(), redisKey).Result()
	require.NoError(t, err)
	assert.Greater(t, ttl, time.Duration(0))
	assert.LessOrEqual(t, ttl, 30*time.Second)

	// Start real HTTP test server and upgrade WebSocket with allowed Tauri/Web origin
	ts := httptest.NewServer(env.handler)
	defer ts.Close()

	wsTarget := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws?ticket=" + ticketResp.Ticket
	wsHdr := http.Header{"Origin": {"http://localhost:5173"}}
	conn, resp, err := websocket.DefaultDialer.Dial(wsTarget, wsHdr)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	// Verify READY payload includes userId, guildId, and heartbeatIntervalMs
	readyFrame := readWSFrame(t, conn, 3*time.Second)
	assert.Equal(t, gateway.EventReady, readyFrame.T)

	var ready gateway.ReadyPayload
	require.NoError(t, json.Unmarshal(readyFrame.D, &ready))
	assert.Equal(t, user.User.ID, ready.UserID)
	assert.Contains(t, ready.GuildIDs, guild.ID)
	assert.Equal(t, int64(25000), ready.HeartbeatIntervalMs)

	// Send HEARTBEAT -> receive HEARTBEAT_ACK
	require.NoError(t, conn.WriteJSON(gateway.OutboundFrame{T: gateway.EventHeartbeat}))
	ackFrame := readWSFrame(t, conn, 3*time.Second)
	assert.Equal(t, gateway.EventHeartbeatAck, ackFrame.T)

	// Verify ticket key was deleted by GETDEL and replaying the exact same ticket yields HTTP 401
	exists, err := env.redis.Exists(context.Background(), redisKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), exists)

	_, replayResp, replayErr := websocket.DefaultDialer.Dial(wsTarget, wsHdr)
	require.Error(t, replayErr)
	require.NotNil(t, replayResp)
	_ = replayResp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, replayResp.StatusCode)
}

func TestIntegration_TC_WS_02_MessageRealtimeAndNonceAndMsg03(t *testing.T) {
	env := setupIntegrationEnv(t)
	owner := env.registerUser(t, "ws02_owner")
	member := env.registerUser(t, "ws02_member")
	ownerHdr := map[string]string{"Authorization": "Bearer " + owner.AccessToken}
	memberHdr := map[string]string{"Authorization": "Bearer " + member.AccessToken}

	guild, ch := createGuildWithGeneralChannel(t, env, ownerHdr, "WS02 Realtime Guild")

	// Invite and join member BEFORE connecting WS so both start subscribed.
	rec := env.do(t, http.MethodPost, "/api/v1/guilds/"+guild.ID.String()+"/invites", map[string]any{}, ownerHdr, "")
	require.Equal(t, http.StatusCreated, rec.Code)
	var inv dto.InviteResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &inv))
	rec = env.do(t, http.MethodPost, "/api/v1/invites/"+inv.Code+"/accept", nil, memberHdr, "")
	require.Equal(t, http.StatusOK, rec.Code)

	ts := httptest.NewServer(env.handler)
	defer ts.Close()

	connA, _ := dialAuthenticatedWS(t, ts, owner.AccessToken)
	connB, _ := dialAuthenticatedWS(t, ts, member.AccessToken)

	// Client A sends 20 messages sequentially -> both A and B receive 20 MESSAGE_CREATE frames in order with nonce.
	sentIDs := make([]uuid.UUID, 20)
	for i := range 20 {
		nonce := fmt.Sprintf("ws02-nonce-%02d", i)
		var created dto.MessageResponse
		httpJSON(
			t,
			http.MethodPost,
			ts.URL+"/api/v1/channels/"+ch.ID.String()+"/messages",
			owner.AccessToken,
			map[string]any{"content": fmt.Sprintf("tin realtime #%02d", i), "nonce": nonce},
			http.StatusCreated,
			&created,
		)
		sentIDs[i] = created.ID

		for _, conn := range []*websocket.Conn{connA, connB} {
			frame := readWSFrameOfType(t, conn, gateway.EventMessageCreate, 3*time.Second)
			var ev struct {
				ID        uuid.UUID `json:"id"`
				ChannelID uuid.UUID `json:"channelId"`
				GuildID   uuid.UUID `json:"guildId"`
				Content   string    `json:"content"`
				Nonce     *string   `json:"nonce"`
				Author    struct {
					ID uuid.UUID `json:"id"`
				} `json:"author"`
			}
			require.NoError(t, json.Unmarshal(frame.D, &ev))
			assert.Equal(t, created.ID, ev.ID)
			assert.Equal(t, ch.ID, ev.ChannelID)
			assert.Equal(t, guild.ID, ev.GuildID)
			assert.Equal(t, owner.User.ID, ev.Author.ID)
			require.NotNil(t, ev.Nonce)
			assert.Equal(t, nonce, *ev.Nonce)
		}
	}

	// TC-MSG-03 realtime: Member posts a message -> edits it -> Owner deletes it; both receive UPDATE and DELETE.
	var memberMsg dto.MessageResponse
	httpJSON(
		t,
		http.MethodPost,
		ts.URL+"/api/v1/channels/"+ch.ID.String()+"/messages",
		member.AccessToken,
		map[string]any{"content": "ban dau"},
		http.StatusCreated,
		&memberMsg,
	)
	_ = readWSFrameOfType(t, connA, gateway.EventMessageCreate, 3*time.Second)
	_ = readWSFrameOfType(t, connB, gateway.EventMessageCreate, 3*time.Second)

	var updatedMsg dto.MessageResponse
	httpJSON(
		t,
		http.MethodPatch,
		ts.URL+"/api/v1/channels/"+ch.ID.String()+"/messages/"+memberMsg.ID.String(),
		member.AccessToken,
		map[string]any{"content": "da chinh sua realtime"},
		http.StatusOK,
		&updatedMsg,
	)
	for _, conn := range []*websocket.Conn{connA, connB} {
		frame := readWSFrameOfType(t, conn, gateway.EventMessageUpdate, 3*time.Second)
		var ev struct {
			ID        uuid.UUID `json:"id"`
			ChannelID uuid.UUID `json:"channelId"`
			GuildID   uuid.UUID `json:"guildId"`
			Content   string    `json:"content"`
			EditedAt  *string   `json:"editedAt"`
		}
		require.NoError(t, json.Unmarshal(frame.D, &ev))
		assert.Equal(t, memberMsg.ID, ev.ID)
		assert.Equal(t, "da chinh sua realtime", ev.Content)
		assert.NotNil(t, ev.EditedAt)
	}

	httpJSON(
		t,
		http.MethodDelete,
		ts.URL+"/api/v1/channels/"+ch.ID.String()+"/messages/"+memberMsg.ID.String(),
		owner.AccessToken,
		nil,
		http.StatusNoContent,
		nil,
	)
	for _, conn := range []*websocket.Conn{connA, connB} {
		frame := readWSFrameOfType(t, conn, gateway.EventMessageDelete, 3*time.Second)
		var ev struct {
			ID        uuid.UUID `json:"id"`
			ChannelID uuid.UUID `json:"channelId"`
			GuildID   uuid.UUID `json:"guildId"`
		}
		require.NoError(t, json.Unmarshal(frame.D, &ev))
		assert.Equal(t, memberMsg.ID, ev.ID)
		assert.Equal(t, ch.ID, ev.ChannelID)
		assert.Equal(t, guild.ID, ev.GuildID)
	}
}

func TestIntegration_ChannelRealtimeEvents(t *testing.T) {
	env := setupIntegrationEnv(t)
	owner := env.registerUser(t, "chn_rt_owner")
	ownerHdr := map[string]string{"Authorization": "Bearer " + owner.AccessToken}

	guild, _ := createGuildWithGeneralChannel(t, env, ownerHdr, "Channel Realtime Guild")

	ts := httptest.NewServer(env.handler)
	defer ts.Close()

	conn, _ := dialAuthenticatedWS(t, ts, owner.AccessToken)

	var created dto.ChannelResponse
	httpJSON(
		t,
		http.MethodPost,
		ts.URL+"/api/v1/guilds/"+guild.ID.String()+"/channels",
		owner.AccessToken,
		map[string]any{"name": "backend-rt", "topic": "Redis PubSub"},
		http.StatusCreated,
		&created,
	)
	createFrame := readWSFrameOfType(t, conn, gateway.EventChannelCreate, 3*time.Second)
	var chEvent struct {
		GuildID uuid.UUID           `json:"guildId"`
		Channel dto.ChannelResponse `json:"channel"`
	}
	require.NoError(t, json.Unmarshal(createFrame.D, &chEvent))
	assert.Equal(t, guild.ID, chEvent.GuildID)
	assert.Equal(t, created.ID, chEvent.Channel.ID)
	assert.Equal(t, "backend-rt", chEvent.Channel.Name)

	var updated dto.ChannelResponse
	httpJSON(
		t,
		http.MethodPatch,
		ts.URL+"/api/v1/channels/"+created.ID.String(),
		owner.AccessToken,
		map[string]any{"name": "backend-v2", "topic": nil},
		http.StatusOK,
		&updated,
	)
	updateFrame := readWSFrameOfType(t, conn, gateway.EventChannelUpdate, 3*time.Second)
	require.NoError(t, json.Unmarshal(updateFrame.D, &chEvent))
	assert.Equal(t, "backend-v2", chEvent.Channel.Name)
	assert.Nil(t, chEvent.Channel.Topic)

	httpJSON(
		t,
		http.MethodDelete,
		ts.URL+"/api/v1/channels/"+created.ID.String(),
		owner.AccessToken,
		nil,
		http.StatusNoContent,
		nil,
	)
	deleteFrame := readWSFrameOfType(t, conn, gateway.EventChannelDelete, 3*time.Second)
	require.NoError(t, json.Unmarshal(deleteFrame.D, &chEvent))
	assert.Equal(t, created.ID, chEvent.Channel.ID)
}

func TestIntegration_TC_GLD_03_05_07_MembershipAndGuildDelete(t *testing.T) {
	env := setupIntegrationEnv(t)
	owner := env.registerUser(t, "gld_rt_owner")
	memberB := env.registerUser(t, "gld_rt_b")
	memberC := env.registerUser(t, "gld_rt_c")
	ownerHdr := map[string]string{"Authorization": "Bearer " + owner.AccessToken}

	guild, ch := createGuildWithGeneralChannel(t, env, ownerHdr, "Membership Realtime Guild")

	var inv dto.InviteResponse
	rec := env.do(t, http.MethodPost, "/api/v1/guilds/"+guild.ID.String()+"/invites", map[string]any{}, ownerHdr, "")
	require.Equal(t, http.StatusCreated, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &inv))

	ts := httptest.NewServer(env.handler)
	defer ts.Close()

	// Owner connects before memberB accepts invite; memberB connects BEFORE accepting invite too.
	connOwner, _ := dialAuthenticatedWS(t, ts, owner.AccessToken)
	connB, _ := dialAuthenticatedWS(t, ts, memberB.AccessToken)

	// TC-GLD-03: memberB accepts invite -> Owner (and B via SubscribeAndBroadcast) receive GUILD_MEMBER_ADD.
	var acceptedGuild dto.GuildResponse
	httpJSON(t, http.MethodPost, ts.URL+"/api/v1/invites/"+inv.Code+"/accept", memberB.AccessToken, nil, http.StatusOK, &acceptedGuild)
	assert.Equal(t, guild.ID, acceptedGuild.ID)

	for _, conn := range []*websocket.Conn{connOwner, connB} {
		frame := readWSFrameOfType(t, conn, gateway.EventGuildMemberAdd, 3*time.Second)
		var ev struct {
			GuildID uuid.UUID `json:"guildId"`
			User    struct {
				ID       uuid.UUID `json:"id"`
				Username string    `json:"username"`
			} `json:"user"`
			Role string `json:"role"`
		}
		require.NoError(t, json.Unmarshal(frame.D, &ev))
		assert.Equal(t, guild.ID, ev.GuildID)
		assert.Equal(t, memberB.User.ID, ev.User.ID)
		assert.Equal(t, "member", ev.Role)
	}

	// TC-GLD-05: memberB leaves server -> Owner receives GUILD_MEMBER_REMOVE; subsequent message reaches Owner only, NOT B.
	httpJSON(t, http.MethodDelete, ts.URL+"/api/v1/guilds/"+guild.ID.String()+"/members/@me", memberB.AccessToken, nil, http.StatusNoContent, nil)

	removeFrame := readWSFrameOfType(t, connOwner, gateway.EventGuildMemberRemove, 3*time.Second)
	var rmEv struct {
		GuildID uuid.UUID `json:"guildId"`
		User    struct {
			ID       uuid.UUID `json:"id"`
			Username string    `json:"username"`
		} `json:"user"`
	}
	require.NoError(t, json.Unmarshal(removeFrame.D, &rmEv))
	assert.Equal(t, guild.ID, rmEv.GuildID)
	assert.Equal(t, memberB.User.ID, rmEv.User.ID)
	assert.Equal(t, memberB.User.Username, rmEv.User.Username)

	// Send message after B left -> Owner gets MESSAGE_CREATE; B receives nothing (ADR-007).
	httpJSON(
		t,
		http.MethodPost,
		ts.URL+"/api/v1/channels/"+ch.ID.String()+"/messages",
		owner.AccessToken,
		map[string]any{"content": "after leave secret"},
		http.StatusCreated,
		nil,
	)
	_ = readWSFrameOfType(t, connOwner, gateway.EventMessageCreate, 3*time.Second)
	assertNoWSFrame(t, connB, 250*time.Millisecond)

	// TC-GLD-07: memberC joins, connects WS, Owner deletes guild -> memberC receives GUILD_DELETE.
	httpJSON(t, http.MethodPost, ts.URL+"/api/v1/invites/"+inv.Code+"/accept", memberC.AccessToken, nil, http.StatusOK, nil)
	// Drain GUILD_MEMBER_ADD on connOwner before checking GUILD_DELETE.
	_ = readWSFrameOfType(t, connOwner, gateway.EventGuildMemberAdd, 3*time.Second)

	connC, _ := dialAuthenticatedWS(t, ts, memberC.AccessToken)
	httpJSON(t, http.MethodDelete, ts.URL+"/api/v1/guilds/"+guild.ID.String(), owner.AccessToken, nil, http.StatusNoContent, nil)

	for _, conn := range []*websocket.Conn{connOwner, connC} {
		delFrame := readWSFrameOfType(t, conn, gateway.EventGuildDelete, 3*time.Second)
		var delEv struct {
			GuildID uuid.UUID `json:"guildId"`
		}
		require.NoError(t, json.Unmarshal(delFrame.D, &delEv))
		assert.Equal(t, guild.ID, delEv.GuildID)
	}
}

func TestIntegration_TC_WS_06_CrossInstancePubSub(t *testing.T) {
	env := setupIntegrationEnv(t)
	owner := env.registerUser(t, "ws06_owner")
	member := env.registerUser(t, "ws06_member")
	ownerHdr := map[string]string{"Authorization": "Bearer " + owner.AccessToken}

	guild, ch := createGuildWithGeneralChannel(t, env, ownerHdr, "Cross Instance Guild")

	// Start second independent backend instance sharing Postgres & Redis (simulating :8080 and :8081).
	handler2 := newIntegrationBackendInstance(t, env.cfg, env.pool, env.redis, nil)
	ts1 := httptest.NewServer(env.handler)
	defer ts1.Close()
	ts2 := httptest.NewServer(handler2)
	defer ts2.Close()

	// Create invite on instance 1, accept on instance 2.
	var inv dto.InviteResponse
	httpJSON(t, http.MethodPost, ts1.URL+"/api/v1/guilds/"+guild.ID.String()+"/invites", owner.AccessToken, map[string]any{}, http.StatusCreated, &inv)
	httpJSON(t, http.MethodPost, ts2.URL+"/api/v1/invites/"+inv.Code+"/accept", member.AccessToken, nil, http.StatusOK, nil)

	// Client A connects to instance 1; Client B connects to instance 2.
	connA, _ := dialAuthenticatedWS(t, ts1, owner.AccessToken)
	connB, _ := dialAuthenticatedWS(t, ts2, member.AccessToken)

	// Client A posts 20 messages to instance 1 -> Client B on instance 2 receives all 20 in exact order.
	for i := range 20 {
		nonce := fmt.Sprintf("cross-nonce-%02d", i)
		var created dto.MessageResponse
		httpJSON(
			t,
			http.MethodPost,
			ts1.URL+"/api/v1/channels/"+ch.ID.String()+"/messages",
			owner.AccessToken,
			map[string]any{"content": fmt.Sprintf("cross-instance #%02d", i), "nonce": nonce},
			http.StatusCreated,
			&created,
		)

		frameA := readWSFrameOfType(t, connA, gateway.EventMessageCreate, 3*time.Second)
		require.Equal(t, gateway.EventMessageCreate, frameA.T)

		frameB := readWSFrameOfType(t, connB, gateway.EventMessageCreate, 3*time.Second)
		require.Equal(t, gateway.EventMessageCreate, frameB.T)

		var evB struct {
			ID      uuid.UUID `json:"id"`
			Content string    `json:"content"`
			Nonce   *string   `json:"nonce"`
		}
		require.NoError(t, json.Unmarshal(frameB.D, &evB))
		assert.Equal(t, created.ID, evB.ID)
		assert.Equal(t, created.Content, evB.Content)
		require.NotNil(t, evB.Nonce)
		assert.Equal(t, nonce, *evB.Nonce)
	}
}

func TestIntegration_TC_WS_04_TypingIndicatorAndRateLimit(t *testing.T) {
	env := setupIntegrationEnv(t)
	owner := env.registerUser(t, "ws04_owner")
	member := env.registerUser(t, "ws04_member")
	ownerHdr := map[string]string{"Authorization": "Bearer " + owner.AccessToken}

	guild, ch := createGuildWithGeneralChannel(t, env, ownerHdr, "Typing Guild")
	var inv dto.InviteResponse
	rec := env.do(t, http.MethodPost, "/api/v1/guilds/"+guild.ID.String()+"/invites", map[string]any{}, ownerHdr, "")
	require.Equal(t, http.StatusCreated, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &inv))
	env.do(t, http.MethodPost, "/api/v1/invites/"+inv.Code+"/accept", nil, map[string]string{"Authorization": "Bearer " + member.AccessToken}, "")

	ts := httptest.NewServer(env.handler)
	defer ts.Close()

	connA, _ := dialAuthenticatedWS(t, ts, owner.AccessToken)
	connB, _ := dialAuthenticatedWS(t, ts, member.AccessToken)
	env.trackRedisKey("typing:" + ch.ID.String() + ":" + owner.User.ID.String())
	env.trackRedisKey("presence:" + owner.User.ID.String())
	env.trackRedisKey("presence:" + member.User.ID.String())

	// Send two rapid TYPING_START frames -> Redis SET NX EX 3 allows only the 1st.
	require.NoError(t, connA.WriteJSON(gateway.OutboundFrame{
		T: gateway.EventTypingStart,
		D: gateway.TypingStartPayload{ChannelID: ch.ID},
	}))
	require.NoError(t, connA.WriteJSON(gateway.OutboundFrame{
		T: gateway.EventTypingStart,
		D: gateway.TypingStartPayload{ChannelID: ch.ID},
	}))

	typingFrame := readWSFrameOfType(t, connB, gateway.EventUserTyping, 3*time.Second)
	var typingEv struct {
		ChannelID uuid.UUID `json:"channelId"`
		GuildID   uuid.UUID `json:"guildId"`
		UserID    uuid.UUID `json:"userId"`
		Username  string    `json:"username"`
		Timestamp int64     `json:"timestamp"`
	}
	require.NoError(t, json.Unmarshal(typingFrame.D, &typingEv))
	assert.Equal(t, ch.ID, typingEv.ChannelID)
	assert.Equal(t, guild.ID, typingEv.GuildID)
	assert.Equal(t, owner.User.ID, typingEv.UserID)
	assert.Equal(t, owner.User.Username, typingEv.Username)
	assert.Greater(t, typingEv.Timestamp, int64(0))

	ttl, err := env.redis.TTL(context.Background(), "typing:"+ch.ID.String()+":"+owner.User.ID.String()).Result()
	require.NoError(t, err)
	assert.Greater(t, ttl, time.Duration(0))
	assert.LessOrEqual(t, ttl, 3*time.Second)

	// Confirm the 2nd immediate TYPING_START was throttled.
	assertNoWSFrame(t, connB, 180*time.Millisecond)
}

func TestIntegration_TC_WS_05_07_PresenceMultiTabAndOffline(t *testing.T) {
	env := setupIntegrationEnv(t)
	owner := env.registerUser(t, "ws05_owner")
	memberB := env.registerUser(t, "ws05_b")
	ownerHdr := map[string]string{"Authorization": "Bearer " + owner.AccessToken}

	guild, ch := createGuildWithGeneralChannel(t, env, ownerHdr, "Presence Guild")
	var inv dto.InviteResponse
	rec := env.do(t, http.MethodPost, "/api/v1/guilds/"+guild.ID.String()+"/invites", map[string]any{}, ownerHdr, "")
	require.Equal(t, http.StatusCreated, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &inv))
	env.do(t, http.MethodPost, "/api/v1/invites/"+inv.Code+"/accept", nil, map[string]string{"Authorization": "Bearer " + memberB.AccessToken}, "")

	ts := httptest.NewServer(env.handler)
	defer ts.Close()

	env.trackRedisKey("presence:" + owner.User.ID.String())
	env.trackRedisKey("presence:" + memberB.User.ID.String())

	connOwner, _ := dialAuthenticatedWS(t, ts, owner.AccessToken)
	tab1B, _ := dialAuthenticatedWS(t, ts, memberB.AccessToken)
	tab2B, _ := dialAuthenticatedWS(t, ts, memberB.AccessToken)

	// Owner sees memberB online via PRESENCE_UPDATE.
	deadline := time.Now().Add(3 * time.Second)
	for {
		frame := readWSFrameOfType(t, connOwner, gateway.EventPresenceUpdate, time.Until(deadline))
		var pEv struct {
			UserID uuid.UUID `json:"userId"`
			Status string    `json:"status"`
		}
		require.NoError(t, json.Unmarshal(frame.D, &pEv))
		if pEv.UserID == memberB.User.ID && pEv.Status == "online" {
			break
		}
	}

	ttl, err := env.redis.TTL(context.Background(), "presence:"+memberB.User.ID.String()).Result()
	require.NoError(t, err)
	assert.Greater(t, ttl, time.Duration(0))
	assert.LessOrEqual(t, ttl, 60*time.Second)

	// TC-WS-07: Both tabs of memberB receive messages.
	httpJSON(
		t,
		http.MethodPost,
		ts.URL+"/api/v1/channels/"+ch.ID.String()+"/messages",
		owner.AccessToken,
		map[string]any{"content": "multi tab check"},
		http.StatusCreated,
		nil,
	)
	_ = readWSFrameOfType(t, connOwner, gateway.EventMessageCreate, 3*time.Second)
	_ = readWSFrameOfType(t, tab1B, gateway.EventMessageCreate, 3*time.Second)
	_ = readWSFrameOfType(t, tab2B, gateway.EventMessageCreate, 3*time.Second)

	// Close Tab 1 -> memberB MUST remain online while Tab 2 is still open.
	_ = tab1B.Close()
	time.Sleep(150 * time.Millisecond)

	var members []dto.GuildMemberResponse
	httpJSON(t, http.MethodGet, ts.URL+"/api/v1/guilds/"+guild.ID.String()+"/members", owner.AccessToken, nil, http.StatusOK, &members)
	for _, m := range members {
		if m.UserID == memberB.User.ID {
			assert.True(t, m.Online, "memberB should remain online while tab2 is open")
		}
	}

	// Close Tab 2 (last tab) -> Owner receives PRESENCE_UPDATE offline and GET members shows offline.
	_ = tab2B.Close()
	deadline = time.Now().Add(3 * time.Second)
	for {
		frame := readWSFrameOfType(t, connOwner, gateway.EventPresenceUpdate, time.Until(deadline))
		var pEv struct {
			UserID uuid.UUID `json:"userId"`
			Status string    `json:"status"`
		}
		require.NoError(t, json.Unmarshal(frame.D, &pEv))
		if pEv.UserID == memberB.User.ID {
			assert.Equal(t, "offline", pEv.Status)
			break
		}
	}

	exists, err := env.redis.Exists(context.Background(), "presence:"+memberB.User.ID.String()).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), exists)

	httpJSON(t, http.MethodGet, ts.URL+"/api/v1/guilds/"+guild.ID.String()+"/members", owner.AccessToken, nil, http.StatusOK, &members)
	for _, m := range members {
		if m.UserID == memberB.User.ID {
			assert.False(t, m.Online, "memberB should be offline after last tab closes")
		}
	}
}

func TestIntegration_TC_UI_03_CORSAndWSOrigins(t *testing.T) {
	env := setupIntegrationEnv(t)
	user := env.registerUser(t, "ui03_user")

	ts := httptest.NewServer(env.handler)
	defer ts.Close()
	env.trackRedisKey("presence:" + user.User.ID.String())

	allowedOrigins := []string{
		"http://localhost:5173",
		"http://127.0.0.1:5173",
		"tauri://localhost",
		"http://tauri.localhost",
		"https://tauri.localhost",
	}

	for _, origin := range allowedOrigins {
		// REST CORS header check
		req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/users/@me", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+user.AccessToken)
		req.Header.Set("Origin", origin)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, origin, resp.Header.Get("Access-Control-Allow-Origin"))

		// WS CheckOrigin + ticket upgrade check
		var ticketResp dto.WSTicketResponse
		httpJSON(t, http.MethodPost, ts.URL+"/api/v1/ws/ticket", user.AccessToken, nil, http.StatusOK, &ticketResp)
		wsTarget := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws?ticket=" + ticketResp.Ticket
		conn, wsResp, err := websocket.DefaultDialer.Dial(wsTarget, http.Header{"Origin": {origin}})
		if wsResp != nil {
			_ = wsResp.Body.Close()
		}
		require.NoError(t, err, "expected WS upgrade to succeed for origin %s", origin)
		_ = conn.Close()
	}

	// Untrusted origin must be rejected at WS upgrade (403).
	var ticketResp dto.WSTicketResponse
	httpJSON(t, http.MethodPost, ts.URL+"/api/v1/ws/ticket", user.AccessToken, nil, http.StatusOK, &ticketResp)
	wsTarget := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws?ticket=" + ticketResp.Ticket
	_, badResp, badErr := websocket.DefaultDialer.Dial(wsTarget, http.Header{"Origin": {"https://evil.example.com"}})
	require.Error(t, badErr)
	require.NotNil(t, badResp)
	_ = badResp.Body.Close()
	assert.Equal(t, http.StatusForbidden, badResp.StatusCode)
}
