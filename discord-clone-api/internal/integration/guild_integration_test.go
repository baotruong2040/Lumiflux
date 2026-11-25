package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/truong/discord-clone-api/internal/controller"
	"github.com/truong/discord-clone-api/internal/controller/dto"
)

func (e *testEnv) registerUser(t *testing.T, prefix string) dto.AuthResponse {
	t.Helper()
	suf := uniqueSuffix()
	rec := e.do(t, http.MethodPost, "/api/v1/auth/register", dto.RegisterRequest{
		Username:    prefix + "_" + suf,
		Email:       fmt.Sprintf("%s_%s@example.com", prefix, suf),
		DisplayName: "User " + prefix,
		Password:    "StrongPass123!",
	}, nil, "")
	require.Equal(t, http.StatusCreated, rec.Code)

	var res dto.AuthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	e.trackUser(res.User.ID)
	e.trackRedisKey("refresh:" + e.jwtMgr.HashRefreshToken(res.RefreshToken))
	return res
}

func TestIntegration_TC_GLD_01_CreateAndCRUDGuild(t *testing.T) {
	env := setupIntegrationEnv(t)
	owner := env.registerUser(t, "gld01_owner")
	outsider := env.registerUser(t, "gld01_other")
	ownerHdr := map[string]string{"Authorization": "Bearer " + owner.AccessToken}
	outsiderHdr := map[string]string{"Authorization": "Bearer " + outsider.AccessToken}

	// Create guild -> 201 + DB transaction creates guilds + #general + owner
	rec := env.do(t, http.MethodPost, "/api/v1/guilds", dto.CreateGuildRequest{
		Name: "Lumiflux Guild",
	}, ownerHdr, "")
	require.Equal(t, http.StatusCreated, rec.Code)

	var guild dto.GuildResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &guild))
	env.trackGuild(guild.ID)
	assert.Equal(t, "Lumiflux Guild", guild.Name)
	assert.Equal(t, owner.User.ID, guild.OwnerID)

	// Verify #general channel & owner role exist in DB
	var channelName string
	var channelType int16
	err := env.pool.QueryRow(context.Background(),
		`SELECT name, type FROM channels WHERE guild_id = $1`, guild.ID,
	).Scan(&channelName, &channelType)
	require.NoError(t, err)
	assert.Equal(t, "general", channelName)
	assert.Equal(t, int16(0), channelType)

	var role string
	err = env.pool.QueryRow(context.Background(),
		`SELECT role FROM guild_members WHERE guild_id = $1 AND user_id = $2`, guild.ID, owner.User.ID,
	).Scan(&role)
	require.NoError(t, err)
	assert.Equal(t, "owner", role)

	// GET /guilds returns created guild
	rec = env.do(t, http.MethodGet, "/api/v1/guilds", nil, ownerHdr, "")
	require.Equal(t, http.StatusOK, rec.Code)
	var list []dto.GuildResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Len(t, list, 1)
	assert.Equal(t, guild.ID, list[0].ID)

	// Outsider cannot GET / PATCH / DELETE -> 403 FORBIDDEN
	assert.Equal(t, http.StatusForbidden, env.do(t, http.MethodGet, "/api/v1/guilds/"+guild.ID.String(), nil, outsiderHdr, "").Code)
	assert.Equal(t, http.StatusForbidden, env.do(t, http.MethodPatch, "/api/v1/guilds/"+guild.ID.String(), map[string]any{"name": "X"}, outsiderHdr, "").Code)
	assert.Equal(t, http.StatusForbidden, env.do(t, http.MethodDelete, "/api/v1/guilds/"+guild.ID.String(), nil, outsiderHdr, "").Code)

	// Owner updates guild -> 200
	rec = env.do(t, http.MethodPatch, "/api/v1/guilds/"+guild.ID.String(), map[string]any{
		"name":    "Lumiflux Updated",
		"iconUrl": "https://cdn.example.com/g.png",
	}, ownerHdr, "")
	require.Equal(t, http.StatusOK, rec.Code)
	var updated dto.GuildResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updated))
	assert.Equal(t, "Lumiflux Updated", updated.Name)
	require.NotNil(t, updated.IconURL)
	assert.Equal(t, "https://cdn.example.com/g.png", *updated.IconURL)

	// Owner deletes guild -> 204
	rec = env.do(t, http.MethodDelete, "/api/v1/guilds/"+guild.ID.String(), nil, ownerHdr, "")
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Subsequent GET -> 404
	rec = env.do(t, http.MethodGet, "/api/v1/guilds/"+guild.ID.String(), nil, ownerHdr, "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestIntegration_TC_GLD_02_OwnerCannotLeave(t *testing.T) {
	env := setupIntegrationEnv(t)
	owner := env.registerUser(t, "gld02_owner")
	ownerHdr := map[string]string{"Authorization": "Bearer " + owner.AccessToken}

	rec := env.do(t, http.MethodPost, "/api/v1/guilds", dto.CreateGuildRequest{Name: "Owner Guild"}, ownerHdr, "")
	require.Equal(t, http.StatusCreated, rec.Code)
	var guild dto.GuildResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &guild))
	env.trackGuild(guild.ID)

	rec = env.do(t, http.MethodDelete, "/api/v1/guilds/"+guild.ID.String()+"/members/@me", nil, ownerHdr, "")
	assert.Equal(t, http.StatusForbidden, rec.Code)

	var errResp controller.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "FORBIDDEN", errResp.Error.Code)

	_, unsub := env.subs.snapshot()
	assert.Empty(t, unsub)

	var count int
	err := env.pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM guild_members WHERE guild_id = $1 AND user_id = $2`, guild.ID, owner.User.ID,
	).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestIntegration_TC_GLD_03_InviteJoinMembersAndLeave(t *testing.T) {
	env := setupIntegrationEnv(t)
	owner := env.registerUser(t, "gld03_owner")
	member := env.registerUser(t, "gld03_member")
	ownerHdr := map[string]string{"Authorization": "Bearer " + owner.AccessToken}
	memberHdr := map[string]string{"Authorization": "Bearer " + member.AccessToken}

	// Owner creates guild
	rec := env.do(t, http.MethodPost, "/api/v1/guilds", dto.CreateGuildRequest{Name: "Community Server"}, ownerHdr, "")
	require.Equal(t, http.StatusCreated, rec.Code)
	var guild dto.GuildResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &guild))
	env.trackGuild(guild.ID)

	// Owner creates invite (empty body defaults)
	rec = env.do(t, http.MethodPost, "/api/v1/guilds/"+guild.ID.String()+"/invites", nil, ownerHdr, "")
	require.Equal(t, http.StatusCreated, rec.Code)
	var inv dto.InviteResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &inv))
	assert.NotEmpty(t, inv.Code)
	assert.Equal(t, guild.ID, inv.GuildID)
	assert.Equal(t, 0, inv.Uses)
	assert.NotNil(t, inv.ExpiresAt)

	// Mark member online in Redis presence to verify PresenceReader enrichment
	presenceKey := "presence:" + member.User.ID.String()
	env.trackRedisKey(presenceKey)
	require.NoError(t, env.redis.Set(context.Background(), presenceKey, "online", time.Minute).Err())

	// Second user accepts invite -> 200 Guild
	rec = env.do(t, http.MethodPost, "/api/v1/invites/"+inv.Code+"/accept", nil, memberHdr, "")
	require.Equal(t, http.StatusOK, rec.Code)
	var joined dto.GuildResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &joined))
	assert.Equal(t, guild.ID, joined.ID)

	// Duplicate accept -> 409 ALREADY_MEMBER
	rec = env.do(t, http.MethodPost, "/api/v1/invites/"+inv.Code+"/accept", nil, memberHdr, "")
	assert.Equal(t, http.StatusConflict, rec.Code)

	// Member can also create invite (ADR-005)
	rec = env.do(t, http.MethodPost, "/api/v1/guilds/"+guild.ID.String()+"/invites", map[string]any{"maxUses": 5}, memberHdr, "")
	require.Equal(t, http.StatusCreated, rec.Code)

	// Member cannot update or delete guild -> 403
	assert.Equal(t, http.StatusForbidden, env.do(t, http.MethodPatch, "/api/v1/guilds/"+guild.ID.String(), map[string]any{"name": "No"}, memberHdr, "").Code)
	assert.Equal(t, http.StatusForbidden, env.do(t, http.MethodDelete, "/api/v1/guilds/"+guild.ID.String(), nil, memberHdr, "").Code)

	// GET /guilds/:guildId/members lists both with roles and presence
	rec = env.do(t, http.MethodGet, "/api/v1/guilds/"+guild.ID.String()+"/members", nil, memberHdr, "")
	require.Equal(t, http.StatusOK, rec.Code)
	var members []dto.GuildMemberResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &members))
	require.Len(t, members, 2)
	assert.Equal(t, "owner", members[0].Role)
	assert.Equal(t, owner.User.ID, members[0].UserID)
	assert.Equal(t, "member", members[1].Role)
	assert.Equal(t, member.User.ID, members[1].UserID)
	assert.True(t, members[1].Online)

	// Member leaves server -> 204 and invokes ADR-007 Unsubscribe hook
	rec = env.do(t, http.MethodDelete, "/api/v1/guilds/"+guild.ID.String()+"/members/@me", nil, memberHdr, "")
	assert.Equal(t, http.StatusNoContent, rec.Code)

	sub, unsub := env.subs.snapshot()
	assert.Contains(t, sub, subEvent{userID: owner.User.ID, guildID: guild.ID})
	assert.Contains(t, sub, subEvent{userID: member.User.ID, guildID: guild.ID})
	assert.Contains(t, unsub, subEvent{userID: member.User.ID, guildID: guild.ID})

	// Former member can no longer access guild
	assert.Equal(t, http.StatusForbidden, env.do(t, http.MethodGet, "/api/v1/guilds/"+guild.ID.String(), nil, memberHdr, "").Code)
}

func TestIntegration_TC_GLD_04_InviteExpiredAndExhausted(t *testing.T) {
	env := setupIntegrationEnv(t)
	owner := env.registerUser(t, "gld04_owner")
	userB := env.registerUser(t, "gld04_b")
	userC := env.registerUser(t, "gld04_c")
	ownerHdr := map[string]string{"Authorization": "Bearer " + owner.AccessToken}
	userBHdr := map[string]string{"Authorization": "Bearer " + userB.AccessToken}
	userCHdr := map[string]string{"Authorization": "Bearer " + userC.AccessToken}

	rec := env.do(t, http.MethodPost, "/api/v1/guilds", dto.CreateGuildRequest{Name: "Limit Guild"}, ownerHdr, "")
	require.Equal(t, http.StatusCreated, rec.Code)
	var guild dto.GuildResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &guild))
	env.trackGuild(guild.ID)

	// 1. Expired invite -> 400 INVITE_EXPIRED
	rec = env.do(t, http.MethodPost, "/api/v1/guilds/"+guild.ID.String()+"/invites", map[string]any{"expiresInHours": 1}, ownerHdr, "")
	require.Equal(t, http.StatusCreated, rec.Code)
	var expiredInv dto.InviteResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &expiredInv))

	_, err := env.pool.Exec(context.Background(), `UPDATE invites SET expires_at = now() - interval '5 minutes' WHERE code = $1`, expiredInv.Code)
	require.NoError(t, err)

	rec = env.do(t, http.MethodPost, "/api/v1/invites/"+expiredInv.Code+"/accept", nil, userBHdr, "")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var expErr controller.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &expErr))
	assert.Equal(t, "INVITE_EXPIRED", expErr.Error.Code)

	// 2. Single-use invite -> first succeeds, second fails with 400 INVITE_EXHAUSTED
	rec = env.do(t, http.MethodPost, "/api/v1/guilds/"+guild.ID.String()+"/invites", map[string]any{"maxUses": 1}, ownerHdr, "")
	require.Equal(t, http.StatusCreated, rec.Code)
	var limitedInv dto.InviteResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &limitedInv))

	rec = env.do(t, http.MethodPost, "/api/v1/invites/"+limitedInv.Code+"/accept", nil, userBHdr, "")
	require.Equal(t, http.StatusOK, rec.Code)

	rec = env.do(t, http.MethodPost, "/api/v1/invites/"+limitedInv.Code+"/accept", nil, userCHdr, "")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var exErr controller.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &exErr))
	assert.Equal(t, "INVITE_EXHAUSTED", exErr.Error.Code)

	// 3. Unknown invite code -> 404 INVITE_NOT_FOUND
	rec = env.do(t, http.MethodPost, "/api/v1/invites/UNKNOWN1/accept", nil, userCHdr, "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

var _ = uuid.Nil
