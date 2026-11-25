import type { Message } from '../../core/api'

export type OptimisticStatus = 'sending' | 'failed'

export interface ChatMessage extends Message {
  optimisticStatus?: OptimisticStatus
  errorMessage?: string
}
