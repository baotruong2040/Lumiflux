import type { QueryClient } from '@tanstack/react-query'
import type { Channel, Guild, GuildMember, Message } from '../../core/api'
import type { GatewayEvent } from '../../core/gateway'
import {
  patchMessageInPages,
  removeMessageFromPages,
  upsertIncomingMessage,
} from '../chat/utils/messageHelpers'
import type { MessagesInfiniteData } from '../chat/utils/messageHelpers'

function sortChannels(list: Channel[]): Channel[] {
  return [...list].sort((a, b) => a.position - b.position || a.id.localeCompare(b.id))
}

export function applyGatewayEventToCache(
  queryClient: QueryClient,
  event: GatewayEvent,
  currentUserId: string | null,
): void {
  switch (event.t) {
    case 'MESSAGE_CREATE': {
      const msg: Message = {
        id: event.d.id,
        channelId: event.d.channelId,
        author: event.d.author,
        content: event.d.content,
        nonce: event.d.nonce,
        editedAt: event.d.editedAt,
        createdAt: event.d.createdAt,
      }
      queryClient.setQueryData<MessagesInfiniteData>(['messages', event.d.channelId], (old) =>
        upsertIncomingMessage(old, msg),
      )
      return
    }

    case 'MESSAGE_UPDATE': {
      queryClient.setQueryData<MessagesInfiniteData>(['messages', event.d.channelId], (old) =>
        patchMessageInPages(old, event.d.id, {
          content: event.d.content,
          editedAt: event.d.editedAt,
        }),
      )
      return
    }

    case 'MESSAGE_DELETE': {
      queryClient.setQueryData<MessagesInfiniteData>(['messages', event.d.channelId], (old) =>
        removeMessageFromPages(old, event.d.id),
      )
      return
    }

    case 'CHANNEL_CREATE': {
      const { guildId, channel } = event.d
      queryClient.setQueryData<Channel[]>(['channels', guildId], (old) => {
        if (!old) return old
        if (old.some((c) => c.id === channel.id)) {
          return sortChannels(old.map((c) => (c.id === channel.id ? channel : c)))
        }
        return sortChannels([...old, channel])
      })
      return
    }

    case 'CHANNEL_UPDATE': {
      const { guildId, channel } = event.d
      queryClient.setQueryData<Channel[]>(['channels', guildId], (old) => {
        if (!old) return old
        if (!old.some((c) => c.id === channel.id)) {
          return sortChannels([...old, channel])
        }
        return sortChannels(old.map((c) => (c.id === channel.id ? channel : c)))
      })
      return
    }

    case 'CHANNEL_DELETE': {
      const { guildId, channel } = event.d
      queryClient.setQueryData<Channel[]>(['channels', guildId], (old) =>
        old ? old.filter((c) => c.id !== channel.id) : old,
      )
      queryClient.removeQueries({ queryKey: ['messages', channel.id] })
      return
    }

    case 'GUILD_MEMBER_ADD': {
      const { guildId, user, role, nickname, joinedAt } = event.d
      queryClient.setQueryData<GuildMember[]>(['members', guildId], (old) => {
        if (!old) return old
        if (old.some((m) => m.userId === user.id)) {
          return old
        }
        const added: GuildMember = {
          guildId,
          userId: user.id,
          nickname: nickname ?? null,
          role,
          online: true,
          joinedAt: joinedAt ?? new Date().toISOString(),
          user: {
            id: user.id,
            username: user.username,
            email: '',
            displayName: user.displayName,
            avatarUrl: user.avatarUrl,
            createdAt: joinedAt ?? new Date().toISOString(),
          },
        }
        return [...old, added]
      })
      void queryClient.invalidateQueries({ queryKey: ['members', guildId] })
      if (currentUserId && user.id === currentUserId) {
        void queryClient.invalidateQueries({ queryKey: ['guilds'] })
      }
      return
    }

    case 'GUILD_MEMBER_REMOVE': {
      const { guildId, user } = event.d
      queryClient.setQueryData<GuildMember[]>(['members', guildId], (old) =>
        old ? old.filter((m) => m.userId !== user.id) : old,
      )
      if (currentUserId && user.id === currentUserId) {
        queryClient.setQueryData<Guild[]>(['guilds'], (old) =>
          old ? old.filter((g) => g.id !== guildId) : old,
        )
        queryClient.removeQueries({ queryKey: ['channels', guildId] })
        queryClient.removeQueries({ queryKey: ['members', guildId] })
      }
      return
    }

    case 'GUILD_DELETE': {
      const { guildId } = event.d
      queryClient.setQueryData<Guild[]>(['guilds'], (old) =>
        old ? old.filter((g) => g.id !== guildId) : old,
      )
      queryClient.removeQueries({ queryKey: ['channels', guildId] })
      queryClient.removeQueries({ queryKey: ['members', guildId] })
      return
    }

    case 'PRESENCE_UPDATE': {
      const online = event.d.status === 'online'
      queryClient.setQueriesData<GuildMember[]>({ queryKey: ['members'] }, (old) =>
        old
          ? old.map((m) => (m.userId === event.d.userId ? { ...m, online } : m))
          : old,
      )
      return
    }

    case 'READY':
    case 'HEARTBEAT_ACK':
    case 'ERROR':
    case 'USER_TYPING':
      return
  }
}

export function handleGatewayResync(queryClient: QueryClient): void {
  void queryClient.invalidateQueries({ queryKey: ['guilds'] })
  void queryClient.invalidateQueries({ queryKey: ['channels'] })
  void queryClient.invalidateQueries({ queryKey: ['members'] })
  void queryClient.invalidateQueries({ queryKey: ['messages'] })
}
