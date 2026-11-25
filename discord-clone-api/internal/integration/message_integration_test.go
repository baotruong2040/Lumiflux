package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/truong/discord-clone-api/internal/controller"
	"github.com/truong/discord-clone-api/internal/controller/dto"
	"github.com/truong/discord-clone-api/pkg/idgen"
)

func createGuildWithGeneralChannel(t *testing.T, env *testEnv, ownerHdr map[string]string, name string) (dto.GuildResponse, dto.ChannelResponse) {
	t.Helper()

	rec := env.do(t, http.MethodPost, "/api/v1/guilds", dto.CreateGuildRequest{Name: name}, ownerHdr, "")
	require.Equal(t, http.StatusCreated, rec.Code)
	var guild dto.GuildResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &guild))
	env.trackGuild(guild.ID)

	rec = env.do(t, http.MethodGet, "/api/v1/guilds/"+guild.ID.String()+"/channels", nil, ownerHdr, "")
	require.Equal(t, http.StatusOK, rec.Code)
	var channels []dto.ChannelResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &channels))
	require.NotEmpty(t, channels)

	return guild, channels[0]
}

func TestIntegration_TC_MSG_01_Validation(t *testing.T) {
	env := setupIntegrationEnv(t)
	owner := env.registerUser(t, "msg01_owner")
	ownerHdr := map[string]string{"Authorization": "Bearer " + owner.AccessToken}
	_, ch := createGuildWithGeneralChannel(t, env, ownerHdr, "Msg01 Guild")

	// Empty / whitespace -> 400
	rec := env.do(t, http.MethodPost, "/api/v1/channels/"+ch.ID.String()+"/messages", dto.SendMessageRequest{
		Content: "   \n\t ",
	}, ownerHdr, "")
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// 2001 characters -> 400
	rec = env.do(t, http.MethodPost, "/api/v1/channels/"+ch.ID.String()+"/messages", dto.SendMessageRequest{
		Content: strings.Repeat("ă", 2001),
	}, ownerHdr, "")
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// Nonce > 64 -> 400
	longNonce := strings.Repeat("n", 65)
	rec = env.do(t, http.MethodPost, "/api/v1/channels/"+ch.ID.String()+"/messages", dto.SendMessageRequest{
		Content: "Valid",
		Nonce:   &longNonce,
	}, ownerHdr, "")
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// Exact 2000 characters succeeds -> 201
	nonce := "nonce-2000"
	rec = env.do(t, http.MethodPost, "/api/v1/channels/"+ch.ID.String()+"/messages", dto.SendMessageRequest{
		Content: strings.Repeat("á", 2000),
		Nonce:   &nonce,
	}, ownerHdr, "")
	require.Equal(t, http.StatusCreated, rec.Code)
	var created dto.MessageResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.Len(t, []rune(created.Content), 2000)
	require.NotNil(t, created.Nonce)
	assert.Equal(t, nonce, *created.Nonce)
	env.trackRedisKey("rl:" + owner.User.ID.String() + ":msg")
}

func TestIntegration_TC_MSG_02_Seed1000CursorPagination(t *testing.T) {
	env := setupIntegrationEnv(t)
	owner := env.registerUser(t, "msg02_owner")
	ownerHdr := map[string]string{"Authorization": "Bearer " + owner.AccessToken}
	_, ch := createGuildWithGeneralChannel(t, env, ownerHdr, "Seed1000 Guild")

	// Seed 1,000 messages directly with monotonic UUIDv7 identifiers.
	const total = 1000
	rows := make([][]any, 0, total)
	expectedIDs := make([]uuid.UUID, 0, total)
	baseTime := time.Now().UTC().Add(-time.Duration(total) * time.Second)
	for i := 1; i <= total; i++ {
		id, err := idgen.NewUUIDv7()
		require.NoError(t, err)
		if i > 1 {
			require.Greater(t, bytes.Compare(id[:], expectedIDs[len(expectedIDs)-1][:]), 0)
		}
		expectedIDs = append(expectedIDs, id)
		rows = append(rows, []any{id, ch.ID, owner.User.ID, fmt.Sprintf("Seeded #%04d", i), baseTime.Add(time.Duration(i) * time.Second)})
	}

	copied, err := env.pool.CopyFrom(
		context.Background(),
		pgx.Identifier{"messages"},
		[]string{"id", "channel_id", "author_id", "content", "created_at"},
		pgx.CopyFromRows(rows),
	)
	require.NoError(t, err)
	require.Equal(t, int64(total), copied)

	// Paginate through all 20 pages of 50 without duplicates or gaps.
	seen := make(map[uuid.UUID]struct{}, total)
	collected := make([]dto.MessageResponse, 0, total)
	var beforeCursor *uuid.UUID

	for pageNum := range 20 {
		path := fmt.Sprintf("/api/v1/channels/%s/messages?limit=50", ch.ID.String())
		if beforeCursor != nil {
			path += "&before=" + beforeCursor.String()
		}
		rec := env.do(t, http.MethodGet, path, nil, ownerHdr, "")
		require.Equal(t, http.StatusOK, rec.Code)

		var page []dto.MessageResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page))
		require.Len(t, page, 50, "page %d should contain 50 items", pageNum+1)

		for _, msg := range page {
			_, dup := seen[msg.ID]
			require.False(t, dup, "duplicate message id %s", msg.ID)
			seen[msg.ID] = struct{}{}
			collected = append(collected, msg)
		}
		lastID := page[len(page)-1].ID
		beforeCursor = &lastID
	}

	// Next page after the oldest message must be empty.
	rec := env.do(t, http.MethodGet, fmt.Sprintf("/api/v1/channels/%s/messages?limit=50&before=%s", ch.ID.String(), beforeCursor.String()), nil, ownerHdr, "")
	require.Equal(t, http.StatusOK, rec.Code)
	var emptyPage []dto.MessageResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &emptyPage))
	assert.Empty(t, emptyPage)

	require.Len(t, collected, total)
	assert.Equal(t, "Seeded #1000", collected[0].Content)
	assert.Equal(t, "Seeded #0001", collected[len(collected)-1].Content)
	for i := range expectedIDs {
		assert.Equal(t, expectedIDs[total-1-i], collected[i].ID)
	}
}

func TestIntegration_TC_MSG_03_And_TC_GLD_06_EditDeleteAndAdminRBAC(t *testing.T) {
	env := setupIntegrationEnv(t)
	owner := env.registerUser(t, "msg03_owner")
	user2 := env.registerUser(t, "msg03_user2")
	ownerHdr := map[string]string{"Authorization": "Bearer " + owner.AccessToken}
	user2Hdr := map[string]string{"Authorization": "Bearer " + user2.AccessToken}
	guild, ch := createGuildWithGeneralChannel(t, env, ownerHdr, "Msg03 Guild")

	env.trackRedisKey("rl:" + owner.User.ID.String() + ":msg")
	env.trackRedisKey("rl:" + user2.User.ID.String() + ":msg")

	// User2 joins as regular member
	rec := env.do(t, http.MethodPost, "/api/v1/guilds/"+guild.ID.String()+"/invites", nil, ownerHdr, "")
	require.Equal(t, http.StatusCreated, rec.Code)
	var inv dto.InviteResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &inv))
	require.Equal(t, http.StatusOK, env.do(t, http.MethodPost, "/api/v1/invites/"+inv.Code+"/accept", nil, user2Hdr, "").Code)

	// Member sends message, owner sends message
	rec = env.do(t, http.MethodPost, "/api/v1/channels/"+ch.ID.String()+"/messages", dto.SendMessageRequest{Content: "Member msg"}, user2Hdr, "")
	require.Equal(t, http.StatusCreated, rec.Code)
	var memberMsg dto.MessageResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &memberMsg))

	rec = env.do(t, http.MethodPost, "/api/v1/channels/"+ch.ID.String()+"/messages", dto.SendMessageRequest{Content: "Owner msg"}, ownerHdr, "")
	require.Equal(t, http.StatusCreated, rec.Code)
	var ownerMsg dto.MessageResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ownerMsg))

	// Author can edit; owner cannot edit member's message
	assert.Equal(t, http.StatusForbidden, env.do(t, http.MethodPatch, "/api/v1/channels/"+ch.ID.String()+"/messages/"+memberMsg.ID.String(), dto.UpdateMessageRequest{Content: "nope"}, ownerHdr, "").Code)

	rec = env.do(t, http.MethodPatch, "/api/v1/channels/"+ch.ID.String()+"/messages/"+memberMsg.ID.String(), dto.UpdateMessageRequest{Content: "Member edited"}, user2Hdr, "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &memberMsg))
	assert.Equal(t, "Member edited", memberMsg.Content)
	assert.NotNil(t, memberMsg.EditedAt)

	// Member cannot delete owner's message -> 403
	assert.Equal(t, http.StatusForbidden, env.do(t, http.MethodDelete, "/api/v1/channels/"+ch.ID.String()+"/messages/"+ownerMsg.ID.String(), nil, user2Hdr, "").Code)

	// TC-GLD-06: Promote user2 to admin via SQL -> can POST channel & delete member's message (create another member msg first)
	_, err := env.pool.Exec(context.Background(), `UPDATE guild_members SET role = 'admin' WHERE guild_id = $1 AND user_id = $2`, guild.ID, user2.User.ID)
	require.NoError(t, err)

	rec = env.do(t, http.MethodPost, "/api/v1/guilds/"+guild.ID.String()+"/channels", dto.CreateChannelRequest{Name: "admin-room"}, user2Hdr, "")
	require.Equal(t, http.StatusCreated, rec.Code)

	// Admin soft-deletes owner's message -> 204
	rec = env.do(t, http.MethodDelete, "/api/v1/channels/"+ch.ID.String()+"/messages/"+ownerMsg.ID.String(), nil, user2Hdr, "")
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Verify deleted_at is stamped in DB and omitted from GET /messages
	var deletedAt *time.Time
	require.NoError(t, env.pool.QueryRow(context.Background(), `SELECT deleted_at FROM messages WHERE id = $1`, ownerMsg.ID).Scan(&deletedAt))
	assert.NotNil(t, deletedAt)

	rec = env.do(t, http.MethodGet, "/api/v1/channels/"+ch.ID.String()+"/messages", nil, ownerHdr, "")
	require.Equal(t, http.StatusOK, rec.Code)
	var list []dto.MessageResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Len(t, list, 1)
	assert.Equal(t, memberMsg.ID, list[0].ID)

	// Author self-deletes -> 204
	assert.Equal(t, http.StatusNoContent, env.do(t, http.MethodDelete, "/api/v1/channels/"+ch.ID.String()+"/messages/"+memberMsg.ID.String(), nil, user2Hdr, "").Code)
}

func TestIntegration_TC_MSG_04_RateLimit30Per10s(t *testing.T) {
	env := setupIntegrationEnv(t)
	owner := env.registerUser(t, "msg04_owner")
	ownerHdr := map[string]string{"Authorization": "Bearer " + owner.AccessToken}
	_, ch := createGuildWithGeneralChannel(t, env, ownerHdr, "Msg04 Guild")

	rlKey := "rl:" + owner.User.ID.String() + ":msg"
	env.trackRedisKey(rlKey)

	// 30 messages succeed with 201 Created.
	for i := 1; i <= 30; i++ {
		rec := env.do(t, http.MethodPost, "/api/v1/channels/"+ch.ID.String()+"/messages", dto.SendMessageRequest{
			Content: fmt.Sprintf("Burst %d", i),
		}, ownerHdr, "")
		require.Equal(t, http.StatusCreated, rec.Code, "message %d should succeed", i)
	}

	// 31st message returns 429 RATE_LIMITED with Retry-After header.
	rec := env.do(t, http.MethodPost, "/api/v1/channels/"+ch.ID.String()+"/messages", dto.SendMessageRequest{
		Content: "Burst 31",
	}, ownerHdr, "")
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
	var errResp controller.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "RATE_LIMITED", errResp.Error.Code)

	ttl, err := env.redis.TTL(context.Background(), rlKey).Result()
	require.NoError(t, err)
	assert.Greater(t, ttl, time.Duration(0))
	assert.LessOrEqual(t, ttl, 10*time.Second)

	// Verify 30 messages remain intact in DB.
	var count int
	require.NoError(t, env.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM messages WHERE channel_id = $1 AND deleted_at IS NULL`, ch.ID).Scan(&count))
	assert.Equal(t, 30, count)
}
