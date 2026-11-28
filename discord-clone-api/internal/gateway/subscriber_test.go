package gateway

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/truong/discord-clone-api/pkg/idgen"
	"github.com/truong/discord-clone-api/pkg/logger"
	"go.uber.org/goleak"
)

func TestSubscriberDispatch_OrderedGuildAndMemberActions(t *testing.T) {
	t.Cleanup(func() {
		goleak.VerifyNone(t)
	})

	ctx, cancel := context.WithCancel(context.Background())
	log := logger.NewWithWriter(io.Discard, "error")
	hub := NewHub(25*time.Second, log)
	hubDone := make(chan struct{})
	go func() {
		defer close(hubDone)
		hub.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-hubDone
	})

	sub := NewSubscriber(nil, hub, log)
	guildID := idgen.MustUUIDv7()
	ownerID := idgen.MustUUIDv7()
	memberID := idgen.MustUUIDv7()
	joinerID := idgen.MustUUIDv7()

	ownerClient := NewClient(hub, ownerID, []uuid.UUID{guildID}, nil, ClientOptions{Logger: log})
	memberClient := NewClient(hub, memberID, []uuid.UUID{guildID}, nil, ClientOptions{Logger: log})
	joinerClient := NewClient(hub, joinerID, nil, nil, ClientOptions{Logger: log})

	require.True(t, hub.Register(ownerClient))
	require.True(t, hub.Register(memberClient))
	require.True(t, hub.Register(joinerClient))

	// Drain initial READY frames.
	_ = <-ownerClient.send
	_ = <-memberClient.send
	_ = <-joinerClient.send

	// 1. MESSAGE_CREATE broadcasts to current guild members only.
	msgFrame, err := MarshalFrame(EventMessageCreate, map[string]any{
		"id":      idgen.MustUUIDv7().String(),
		"guildId": guildID.String(),
		"content": "hello",
	})
	require.NoError(t, err)
	sub.dispatchMessage(ctx, guildChannelPrefix+guildID.String(), msgFrame)

	assert.Contains(t, string(<-ownerClient.send), `"hello"`)
	assert.Contains(t, string(<-memberClient.send), `"hello"`)
	select {
	case unexpected := <-joinerClient.send:
		t.Fatalf("unexpected frame for non-member joiner: %s", string(unexpected))
	case <-time.After(30 * time.Millisecond):
	}

	// 2. GUILD_MEMBER_ADD subscribes already-connected joiner before fanout.
	addFrame, err := MarshalFrame(EventGuildMemberAdd, map[string]any{
		"guildId": guildID.String(),
		"user": map[string]any{
			"id":       joinerID.String(),
			"username": "joiner",
		},
	})
	require.NoError(t, err)
	sub.dispatchMessage(ctx, guildChannelPrefix+guildID.String(), addFrame)

	assert.Contains(t, string(<-ownerClient.send), EventGuildMemberAdd)
	assert.Contains(t, string(<-memberClient.send), EventGuildMemberAdd)
	assert.Contains(t, string(<-joinerClient.send), EventGuildMemberAdd)

	// 3. GUILD_MEMBER_REMOVE unsubscribes removed member before fanout.
	removeFrame, err := MarshalFrame(EventGuildMemberRemove, map[string]any{
		"guildId": guildID.String(),
		"user": map[string]any{
			"id":       memberID.String(),
			"username": "member",
		},
	})
	require.NoError(t, err)
	sub.dispatchMessage(ctx, guildChannelPrefix+guildID.String(), removeFrame)

	assert.Contains(t, string(<-ownerClient.send), EventGuildMemberRemove)
	assert.Contains(t, string(<-joinerClient.send), EventGuildMemberRemove)
	select {
	case unexpected := <-memberClient.send:
		t.Fatalf("removed member must not receive GUILD_MEMBER_REMOVE fanout: %s", string(unexpected))
	case <-time.After(30 * time.Millisecond):
	}

	// 4. GUILD_DELETE fans out then removes guild from Hub.
	deleteFrame, err := json.Marshal(OutboundFrame{
		T: EventGuildDelete,
		D: map[string]string{"guildId": guildID.String()},
	})
	require.NoError(t, err)
	sub.dispatchMessage(ctx, guildChannelPrefix+guildID.String(), deleteFrame)

	assert.Contains(t, string(<-ownerClient.send), EventGuildDelete)
	assert.Contains(t, string(<-joinerClient.send), EventGuildDelete)

	// Drain any remaining actor work via Stats() and verify guild map is empty.
	stats := hub.Stats()
	assert.Equal(t, 0, stats.Guilds)
}
