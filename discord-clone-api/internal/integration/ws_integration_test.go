package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/truong/discord-clone-api/internal/controller/dto"
	"github.com/truong/discord-clone-api/internal/gateway"
)

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
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, rawReady, err := conn.ReadMessage()
	require.NoError(t, err)

	var readyFrame gateway.InboundFrame
	require.NoError(t, json.Unmarshal(rawReady, &readyFrame))
	assert.Equal(t, gateway.EventReady, readyFrame.T)

	var ready gateway.ReadyPayload
	require.NoError(t, json.Unmarshal(readyFrame.D, &ready))
	assert.Equal(t, user.User.ID, ready.UserID)
	assert.Contains(t, ready.GuildIDs, guild.ID)
	assert.Equal(t, int64(25000), ready.HeartbeatIntervalMs)

	// Send HEARTBEAT -> receive HEARTBEAT_ACK
	require.NoError(t, conn.WriteJSON(gateway.OutboundFrame{T: gateway.EventHeartbeat}))
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, rawAck, err := conn.ReadMessage()
	require.NoError(t, err)
	var ackFrame gateway.InboundFrame
	require.NoError(t, json.Unmarshal(rawAck, &ackFrame))
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
