import { httpClient } from './httpClient'
import type { WSTicketResponse } from './types'

export const wsApi = {
  createTicket(): Promise<WSTicketResponse> {
    return httpClient.post<WSTicketResponse>('/api/v1/ws/ticket')
  },
}
