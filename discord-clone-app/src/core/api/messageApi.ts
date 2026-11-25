import { httpClient } from './httpClient'
import type {
  ListMessagesParams,
  Message,
  SendMessageRequest,
  UpdateMessageRequest,
} from './types'

export const messageApi = {
  listMessages(channelId: string, params?: ListMessagesParams): Promise<Message[]> {
    const query = new URLSearchParams()
    if (params?.before) {
      query.set('before', params.before)
    }
    if (params?.limit !== undefined) {
      query.set('limit', String(params.limit))
    }
    const qs = query.toString()
    const path = qs
      ? `/api/v1/channels/${channelId}/messages?${qs}`
      : `/api/v1/channels/${channelId}/messages`
    return httpClient.get<Message[]>(path)
  },

  sendMessage(channelId: string, payload: SendMessageRequest): Promise<Message> {
    return httpClient.post<Message>(`/api/v1/channels/${channelId}/messages`, payload)
  },

  updateMessage(
    channelId: string,
    messageId: string,
    payload: UpdateMessageRequest,
  ): Promise<Message> {
    return httpClient.patch<Message>(
      `/api/v1/channels/${channelId}/messages/${messageId}`,
      payload,
    )
  },

  deleteMessage(channelId: string, messageId: string): Promise<void> {
    return httpClient.delete<void>(`/api/v1/channels/${channelId}/messages/${messageId}`)
  },
}
