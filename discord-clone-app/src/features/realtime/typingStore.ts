import { create } from 'zustand'

export const TYPING_INDICATOR_TTL_MS = 3500

export interface TypingEntry {
  userId: string
  username: string
  expiresAt: number
}

interface TypingState {
  byChannel: Record<string, TypingEntry[]>
  recordTyping: (channelId: string, userId: string, username: string, ttlMs?: number) => void
  clearUserTyping: (channelId: string, userId: string) => void
  clearChannel: (channelId: string) => void
  clearAll: () => void
}

const expiryTimers = new Map<string, ReturnType<typeof setTimeout>>()

function timerKey(channelId: string, userId: string): string {
  return `${channelId}:${userId}`
}

function clearTimer(key: string): void {
  const timer = expiryTimers.get(key)
  if (timer !== undefined) {
    clearTimeout(timer)
    expiryTimers.delete(key)
  }
}

export const useTypingStore = create<TypingState>((set, get) => ({
  byChannel: {},

  recordTyping(channelId, userId, username, ttlMs = TYPING_INDICATOR_TTL_MS) {
    const key = timerKey(channelId, userId)
    clearTimer(key)

    const expiresAt = Date.now() + ttlMs
    const timer = setTimeout(() => {
      expiryTimers.delete(key)
      get().clearUserTyping(channelId, userId)
    }, ttlMs)
    expiryTimers.set(key, timer)

    set((state) => {
      const existing = state.byChannel[channelId] ?? []
      const filtered = existing.filter((entry) => entry.userId !== userId)
      return {
        byChannel: {
          ...state.byChannel,
          [channelId]: [...filtered, { userId, username, expiresAt }],
        },
      }
    })
  },

  clearUserTyping(channelId, userId) {
    clearTimer(timerKey(channelId, userId))
    set((state) => {
      const existing = state.byChannel[channelId]
      if (!existing || existing.length === 0) {
        return state
      }
      const remaining = existing.filter((entry) => entry.userId !== userId)
      if (remaining.length === existing.length) {
        return state
      }
      if (remaining.length === 0) {
        const next = { ...state.byChannel }
        delete next[channelId]
        return { byChannel: next }
      }
      return {
        byChannel: {
          ...state.byChannel,
          [channelId]: remaining,
        },
      }
    })
  },

  clearChannel(channelId) {
    const prefix = `${channelId}:`
    for (const key of expiryTimers.keys()) {
      if (key.startsWith(prefix)) {
        clearTimer(key)
      }
    }
    set((state) => {
      if (!(channelId in state.byChannel)) {
        return state
      }
      const next = { ...state.byChannel }
      delete next[channelId]
      return { byChannel: next }
    })
  },

  clearAll() {
    for (const key of Array.from(expiryTimers.keys())) {
      clearTimer(key)
    }
    set({ byChannel: {} })
  },
}))

export function formatTypingText(entries: TypingEntry[]): string {
  if (entries.length === 0) {
    return ''
  }
  if (entries.length === 1) {
    return `${entries[0].username} đang nhập...`
  }
  if (entries.length === 2) {
    return `${entries[0].username} và ${entries[1].username} đang nhập...`
  }
  return `${entries[0].username}, ${entries[1].username} và ${entries.length - 2} người khác đang nhập...`
}
