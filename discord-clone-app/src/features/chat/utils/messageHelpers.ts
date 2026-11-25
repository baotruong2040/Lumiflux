import type { InfiniteData } from '@tanstack/react-query'
import { ApiError } from '../../../core/api'
import type { Message } from '../../../core/api'
import type { ChatMessage } from '../types'

export type MessagesInfiniteData = InfiniteData<ChatMessage[], string | undefined>

export function generateNonce(): string {
  if (typeof globalThis.crypto?.randomUUID === 'function') {
    return globalThis.crypto.randomUUID()
  }
  return `n-${Date.now()}-${Math.random().toString(36).slice(2, 12)}`
}

export function upsertOptimisticMessage(
  data: MessagesInfiniteData | undefined,
  optimistic: ChatMessage,
): MessagesInfiniteData {
  if (!data || data.pages.length === 0) {
    return {
      pages: [[optimistic]],
      pageParams: [undefined],
    }
  }

  const nextPages = data.pages.map((page, index) => {
    const filtered = page.filter(
      (item) => !(optimistic.nonce && item.nonce === optimistic.nonce),
    )
    if (index === 0) {
      return [optimistic, ...filtered]
    }
    return filtered
  })

  return {
    ...data,
    pages: nextPages,
  }
}

export function resolveOptimisticMessage(
  data: MessagesInfiniteData | undefined,
  nonce: string,
  serverMsg: Message,
): MessagesInfiniteData {
  const confirmed: ChatMessage = {
    ...serverMsg,
    optimisticStatus: undefined,
    errorMessage: undefined,
  }

  if (!data || data.pages.length === 0) {
    return {
      pages: [[confirmed]],
      pageParams: [undefined],
    }
  }

  let replaced = false
  const nextPages = data.pages.map((page) =>
    page.map((item) => {
      if (item.id === confirmed.id || (item.nonce && item.nonce === nonce)) {
        replaced = true
        return confirmed
      }
      return item
    }),
  )

  if (!replaced) {
    nextPages[0] = [confirmed, ...nextPages[0]]
  }

  return {
    ...data,
    pages: nextPages,
  }
}

export function markOptimisticFailed(
  data: MessagesInfiniteData | undefined,
  nonce: string,
  errorMessage: string,
): MessagesInfiniteData | undefined {
  if (!data) return data

  return {
    ...data,
    pages: data.pages.map((page) =>
      page.map((item) =>
        item.nonce === nonce
          ? {
              ...item,
              optimisticStatus: 'failed',
              errorMessage,
            }
          : item,
      ),
    ),
  }
}

export function updateMessageInPages(
  data: MessagesInfiniteData | undefined,
  updated: Message,
): MessagesInfiniteData | undefined {
  if (!data) return data

  return {
    ...data,
    pages: data.pages.map((page) =>
      page.map((item) =>
        item.id === updated.id
          ? {
              ...updated,
              optimisticStatus: undefined,
              errorMessage: undefined,
            }
          : item,
      ),
    ),
  }
}

export function removeMessageFromPages(
  data: MessagesInfiniteData | undefined,
  idOrNonce: string,
): MessagesInfiniteData | undefined {
  if (!data) return data

  return {
    ...data,
    pages: data.pages.map((page) =>
      page.filter((item) => item.id !== idOrNonce && item.nonce !== idOrNonce),
    ),
  }
}

export function flattenPagesChronologically(pages: ChatMessage[][] | undefined): ChatMessage[] {
  if (!pages || pages.length === 0) {
    return []
  }

  const ordered: ChatMessage[] = []
  const seenIds = new Set<string>()
  const seenNonces = new Set<string>()

  for (let p = pages.length - 1; p >= 0; p--) {
    const page = pages[p]
    for (let i = page.length - 1; i >= 0; i--) {
      const msg = page[i]
      if (seenIds.has(msg.id)) {
        continue
      }
      if (msg.nonce && seenNonces.has(msg.nonce)) {
        continue
      }
      seenIds.add(msg.id)
      if (msg.nonce) {
        seenNonces.add(msg.nonce)
      }
      ordered.push(msg)
    }
  }

  return ordered
}

export function formatMessageTimestamp(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) {
    return ''
  }

  const now = new Date()
  const sameDay =
    date.getFullYear() === now.getFullYear() &&
    date.getMonth() === now.getMonth() &&
    date.getDate() === now.getDate()

  if (sameDay) {
    return date.toLocaleTimeString('vi-VN', {
      hour: '2-digit',
      minute: '2-digit',
    })
  }

  return date.toLocaleString('vi-VN', {
    day: '2-digit',
    month: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export function formatChatError(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.status === 429 || err.code === 'RATE_LIMITED') {
      const wait = err.retryAfter ? ` Thử lại sau ${err.retryAfter} giây.` : ''
      return `Bạn đang gửi tin nhắn quá nhanh (tối đa 30 tin/10 giây).${wait}`
    }
    switch (err.code) {
      case 'FORBIDDEN':
        return 'Bạn không có quyền thực hiện thao tác trên tin nhắn này.'
      case 'MESSAGE_NOT_FOUND':
        return 'Tin nhắn không tồn tại hoặc đã bị xóa.'
      case 'CHANNEL_NOT_FOUND':
        return 'Kênh không tồn tại hoặc đã bị xóa.'
      case 'VALIDATION_ERROR':
        return err.message || 'Nội dung tin nhắn không hợp lệ (1–2.000 ký tự).'
      default:
        return err.message || 'Không thể xử lý yêu cầu tin nhắn.'
    }
  }
  if (err instanceof Error && err.message) {
    return err.message
  }
  return 'Không thể kết nối tới máy chủ.'
}
