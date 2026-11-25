import { httpClient } from './httpClient'
import type {
  CreateGuildRequest,
  CreateInviteRequest,
  Guild,
  GuildMember,
  Invite,
  UpdateGuildRequest,
} from './types'

export const guildApi = {
  listGuilds(): Promise<Guild[]> {
    return httpClient.get<Guild[]>('/api/v1/guilds')
  },

  createGuild(payload: CreateGuildRequest): Promise<Guild> {
    return httpClient.post<Guild>('/api/v1/guilds', payload)
  },

  getGuild(guildId: string): Promise<Guild> {
    return httpClient.get<Guild>(`/api/v1/guilds/${guildId}`)
  },

  updateGuild(guildId: string, payload: UpdateGuildRequest): Promise<Guild> {
    return httpClient.patch<Guild>(`/api/v1/guilds/${guildId}`, payload)
  },

  deleteGuild(guildId: string): Promise<void> {
    return httpClient.delete<void>(`/api/v1/guilds/${guildId}`)
  },

  listMembers(guildId: string): Promise<GuildMember[]> {
    return httpClient.get<GuildMember[]>(`/api/v1/guilds/${guildId}/members`)
  },

  leaveGuild(guildId: string): Promise<void> {
    return httpClient.delete<void>(`/api/v1/guilds/${guildId}/members/@me`)
  },

  createInvite(guildId: string, payload?: CreateInviteRequest): Promise<Invite> {
    return httpClient.post<Invite>(`/api/v1/guilds/${guildId}/invites`, payload ?? {})
  },

  acceptInvite(code: string): Promise<Guild> {
    return httpClient.post<Guild>(`/api/v1/invites/${encodeURIComponent(code)}/accept`)
  },
}
