import { httpClient } from './httpClient'
import type { Channel, CreateChannelRequest, UpdateChannelRequest } from './types'

export const channelApi = {
  listChannels(guildId: string): Promise<Channel[]> {
    return httpClient.get<Channel[]>(`/api/v1/guilds/${guildId}/channels`)
  },

  createChannel(guildId: string, payload: CreateChannelRequest): Promise<Channel> {
    return httpClient.post<Channel>(`/api/v1/guilds/${guildId}/channels`, payload)
  },

  updateChannel(channelId: string, payload: UpdateChannelRequest): Promise<Channel> {
    return httpClient.patch<Channel>(`/api/v1/channels/${channelId}`, payload)
  },

  deleteChannel(channelId: string): Promise<void> {
    return httpClient.delete<void>(`/api/v1/channels/${channelId}`)
  },
}
