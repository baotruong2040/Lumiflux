package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/truong/discord-clone-api/internal/controller"
	"github.com/truong/discord-clone-api/internal/controller/dto"
)

func TestIntegration_TC_CHN_01_MemberForbiddenAdminAndOwnerAllowed(t *testing.T) {
	env := setupIntegrationEnv(t)
	owner := env.registerUser(t, "chn01_owner")
	secondUser := env.registerUser(t, "chn01_user2")
	ownerHdr := map[string]string{"Authorization": "Bearer " + owner.AccessToken}
	user2Hdr := map[string]string{"Authorization": "Bearer " + secondUser.AccessToken}

	// Owner creates guild (includes #general).
	rec := env.do(t, http.MethodPost, "/api/v1/guilds", dto.CreateGuildRequest{Name: "RBAC Channel Guild"}, ownerHdr, "")
	require.Equal(t, http.StatusCreated, rec.Code)
	var guild dto.GuildResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &guild))
	env.trackGuild(guild.ID)

	// GET /guilds/:guildId/channels -> [#general].
	rec = env.do(t, http.MethodGet, "/api/v1/guilds/"+guild.ID.String()+"/channels", nil, ownerHdr, "")
	require.Equal(t, http.StatusOK, rec.Code)
	var channels []dto.ChannelResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &channels))
	require.Len(t, channels, 1)
	assert.Equal(t, "general", channels[0].Name)
	generalID := channels[0].ID

	// Second user joins via invite as regular member.
	rec = env.do(t, http.MethodPost, "/api/v1/guilds/"+guild.ID.String()+"/invites", nil, ownerHdr, "")
	require.Equal(t, http.StatusCreated, rec.Code)
	var inv dto.InviteResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &inv))

	rec = env.do(t, http.MethodPost, "/api/v1/invites/"+inv.Code+"/accept", nil, user2Hdr, "")
	require.Equal(t, http.StatusOK, rec.Code)

	// Member can list channels -> 200.
	rec = env.do(t, http.MethodGet, "/api/v1/guilds/"+guild.ID.String()+"/channels", nil, user2Hdr, "")
	require.Equal(t, http.StatusOK, rec.Code)

	// Member cannot POST / PATCH / DELETE channel -> 403 FORBIDDEN.
	rec = env.do(t, http.MethodPost, "/api/v1/guilds/"+guild.ID.String()+"/channels", dto.CreateChannelRequest{Name: "blocked"}, user2Hdr, "")
	assert.Equal(t, http.StatusForbidden, rec.Code)
	var errResp controller.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "FORBIDDEN", errResp.Error.Code)

	rec = env.do(t, http.MethodPatch, "/api/v1/channels/"+generalID.String(), map[string]any{"name": "hijacked"}, user2Hdr, "")
	assert.Equal(t, http.StatusForbidden, rec.Code)

	rec = env.do(t, http.MethodDelete, "/api/v1/channels/"+generalID.String(), nil, user2Hdr, "")
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// Promote second user to admin via SQL.
	_, err := env.pool.Exec(
		context.Background(),
		`UPDATE guild_members SET role = 'admin' WHERE guild_id = $1 AND user_id = $2`,
		guild.ID,
		secondUser.User.ID,
	)
	require.NoError(t, err)

	// Admin creates channel -> 201.
	topic := "Go Clean Architecture"
	rec = env.do(t, http.MethodPost, "/api/v1/guilds/"+guild.ID.String()+"/channels", dto.CreateChannelRequest{
		Name:  "backend",
		Topic: &topic,
	}, user2Hdr, "")
	require.Equal(t, http.StatusCreated, rec.Code)
	var createdByAdmin dto.ChannelResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createdByAdmin))
	assert.Equal(t, "backend", createdByAdmin.Name)
	require.NotNil(t, createdByAdmin.Topic)
	assert.Equal(t, topic, *createdByAdmin.Topic)
	assert.Equal(t, 1, createdByAdmin.Position)

	// Admin updates name & topic -> 200, then clears topic with null -> 200.
	rec = env.do(t, http.MethodPatch, "/api/v1/channels/"+createdByAdmin.ID.String(), map[string]any{
		"name":  "backend-api",
		"topic": "Updated topic",
	}, user2Hdr, "")
	require.Equal(t, http.StatusOK, rec.Code)
	var updated dto.ChannelResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updated))
	assert.Equal(t, "backend-api", updated.Name)
	require.NotNil(t, updated.Topic)
	assert.Equal(t, "Updated topic", *updated.Topic)

	rec = env.do(t, http.MethodPatch, "/api/v1/channels/"+createdByAdmin.ID.String(), map[string]any{
		"topic": nil,
	}, user2Hdr, "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updated))
	assert.Nil(t, updated.Topic)

	// Admin deletes channel -> 204.
	rec = env.do(t, http.MethodDelete, "/api/v1/channels/"+createdByAdmin.ID.String(), nil, user2Hdr, "")
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Owner creates and deletes channel -> 201 / 204.
	rec = env.do(t, http.MethodPost, "/api/v1/guilds/"+guild.ID.String()+"/channels", dto.CreateChannelRequest{Name: "devops"}, ownerHdr, "")
	require.Equal(t, http.StatusCreated, rec.Code)
	var ownerCh dto.ChannelResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ownerCh))

	rec = env.do(t, http.MethodDelete, "/api/v1/channels/"+ownerCh.ID.String(), nil, ownerHdr, "")
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestIntegration_TC_CHN_02_DuplicateChannelNameConflict(t *testing.T) {
	env := setupIntegrationEnv(t)
	owner := env.registerUser(t, "chn02_owner")
	ownerHdr := map[string]string{"Authorization": "Bearer " + owner.AccessToken}

	rec := env.do(t, http.MethodPost, "/api/v1/guilds", dto.CreateGuildRequest{Name: "Guild One"}, ownerHdr, "")
	require.Equal(t, http.StatusCreated, rec.Code)
	var guild1 dto.GuildResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &guild1))
	env.trackGuild(guild1.ID)

	rec = env.do(t, http.MethodPost, "/api/v1/guilds", dto.CreateGuildRequest{Name: "Guild Two"}, ownerHdr, "")
	require.Equal(t, http.StatusCreated, rec.Code)
	var guild2 dto.GuildResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &guild2))
	env.trackGuild(guild2.ID)

	// Create #announcements in Guild One -> 201.
	rec = env.do(t, http.MethodPost, "/api/v1/guilds/"+guild1.ID.String()+"/channels", dto.CreateChannelRequest{Name: "announcements"}, ownerHdr, "")
	require.Equal(t, http.StatusCreated, rec.Code)

	// Duplicate #announcements in Guild One -> 409 CHANNEL_NAME_TAKEN.
	rec = env.do(t, http.MethodPost, "/api/v1/guilds/"+guild1.ID.String()+"/channels", dto.CreateChannelRequest{Name: "announcements"}, ownerHdr, "")
	assert.Equal(t, http.StatusConflict, rec.Code)
	var errResp controller.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "CHANNEL_NAME_TAKEN", errResp.Error.Code)

	// Duplicate #general in Guild One -> 409 CHANNEL_NAME_TAKEN.
	rec = env.do(t, http.MethodPost, "/api/v1/guilds/"+guild1.ID.String()+"/channels", dto.CreateChannelRequest{Name: "general"}, ownerHdr, "")
	assert.Equal(t, http.StatusConflict, rec.Code)

	// Rename #random to #announcements -> 409 CHANNEL_NAME_TAKEN.
	rec = env.do(t, http.MethodPost, "/api/v1/guilds/"+guild1.ID.String()+"/channels", dto.CreateChannelRequest{Name: "random"}, ownerHdr, "")
	require.Equal(t, http.StatusCreated, rec.Code)
	var randomCh dto.ChannelResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &randomCh))

	rec = env.do(t, http.MethodPatch, "/api/v1/channels/"+randomCh.ID.String(), map[string]any{"name": "announcements"}, ownerHdr, "")
	assert.Equal(t, http.StatusConflict, rec.Code)

	// #announcements in Guild Two is allowed -> 201.
	rec = env.do(t, http.MethodPost, "/api/v1/guilds/"+guild2.ID.String()+"/channels", dto.CreateChannelRequest{Name: "announcements"}, ownerHdr, "")
	assert.Equal(t, http.StatusCreated, rec.Code)
}
