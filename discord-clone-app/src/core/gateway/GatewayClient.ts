import { ApiError, wsApi } from '../api'
import { parseGatewayFrame } from './events'
import type { GatewayEvent, GatewayOutboundFrame } from './events'

export const WS_CLOSE_HEARTBEAT_TIMEOUT = 4003
export const DEFAULT_HEARTBEAT_ACK_TIMEOUT_MS = 10_000
export const DEFAULT_BASE_RECONNECT_DELAY_MS = 1_000
export const DEFAULT_MAX_RECONNECT_DELAY_MS = 30_000

export type GatewayConnectionStatus =
  | 'disconnected'
  | 'connecting'
  | 'connected'
  | 'reconnecting'

export interface GatewaySocketLike {
  readonly readyState: number
  onopen: ((ev: unknown) => void) | null
  onmessage: ((ev: { data: unknown }) => void) | null
  onerror: ((ev: unknown) => void) | null
  onclose: ((ev: { code?: number; reason?: string }) => void) | null
  send(data: string): void
  close(code?: number, reason?: string): void
}

export interface GatewayClientOptions {
  wsUrl: string
  fetchTicket: () => Promise<{ ticket: string }>
  createSocket?: (url: string) => GatewaySocketLike
  heartbeatAckTimeoutMs?: number
  baseReconnectDelayMs?: number
  maxReconnectDelayMs?: number
  random?: () => number
  hasAuthSession?: () => boolean
  logger?: Pick<Console, 'warn'>
}

const WS_READY_STATE_OPEN = 1

export function deriveWsUrl(apiOrWsUrl: string): string {
  const trimmed = apiOrWsUrl.trim().replace(/\/+$/, '')
  if (!trimmed) {
    return 'ws://127.0.0.1:8080/ws'
  }
  let base = trimmed
  if (base.startsWith('https://')) {
    base = `wss://${base.slice('https://'.length)}`
  } else if (base.startsWith('http://')) {
    base = `ws://${base.slice('http://'.length)}`
  }
  return base.endsWith('/ws') ? base : `${base}/ws`
}

export function computeReconnectDelayMs(
  attempt: number,
  opts?: {
    baseDelayMs?: number
    maxDelayMs?: number
    random?: () => number
  },
): number {
  const baseDelayMs = Math.max(50, opts?.baseDelayMs ?? DEFAULT_BASE_RECONNECT_DELAY_MS)
  const maxDelayMs = Math.max(baseDelayMs, opts?.maxDelayMs ?? DEFAULT_MAX_RECONNECT_DELAY_MS)
  const randFn = opts?.random ?? Math.random
  const clampedAttempt = Math.max(0, Math.min(attempt, 16))
  const exponential = Math.min(maxDelayMs, baseDelayMs * 2 ** clampedAttempt)
  // Apply jitter in [0.75, 1.25], clamped between baseDelayMs * 0.5 and maxDelayMs.
  const jittered = Math.round(exponential * (0.75 + randFn() * 0.5))
  return Math.max(Math.round(baseDelayMs * 0.5), Math.min(maxDelayMs, jittered))
}

function createBrowserSocket(url: string): GatewaySocketLike {
  return new WebSocket(url) as unknown as GatewaySocketLike
}

const DEFAULT_WS_BASE_URL = deriveWsUrl(
  (import.meta.env?.VITE_WS_URL as string | undefined) ||
    (import.meta.env?.VITE_API_URL as string | undefined) ||
    'ws://127.0.0.1:8080/ws',
)

export class GatewayClient {
  private readonly wsUrl: string
  private readonly fetchTicket: () => Promise<{ ticket: string }>
  private readonly createSocket: (url: string) => GatewaySocketLike
  private readonly heartbeatAckTimeoutMs: number
  private readonly baseReconnectDelayMs: number
  private readonly maxReconnectDelayMs: number
  private readonly random: () => number
  private readonly hasAuthSession?: () => boolean
  private readonly log: Pick<Console, 'warn'>

  private socket: GatewaySocketLike | null = null
  private status: GatewayConnectionStatus = 'disconnected'
  private intentionallyClosed = true
  private connectionSeq = 0
  private readyCount = 0
  private reconnectAttempt = 0

  private heartbeatTimer: ReturnType<typeof setInterval> | null = null
  private ackTimeoutTimer: ReturnType<typeof setTimeout> | null = null
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null

  private readonly eventListeners = new Set<(event: GatewayEvent) => void>()
  private readonly resyncListeners = new Set<() => void>()
  private readonly statusListeners = new Set<(status: GatewayConnectionStatus) => void>()

  constructor(options: GatewayClientOptions) {
    this.wsUrl = deriveWsUrl(options.wsUrl)
    this.fetchTicket = options.fetchTicket
    this.createSocket = options.createSocket ?? createBrowserSocket
    this.heartbeatAckTimeoutMs = options.heartbeatAckTimeoutMs ?? DEFAULT_HEARTBEAT_ACK_TIMEOUT_MS
    this.baseReconnectDelayMs = options.baseReconnectDelayMs ?? DEFAULT_BASE_RECONNECT_DELAY_MS
    this.maxReconnectDelayMs = options.maxReconnectDelayMs ?? DEFAULT_MAX_RECONNECT_DELAY_MS
    this.random = options.random ?? Math.random
    this.hasAuthSession = options.hasAuthSession
    this.log = options.logger ?? console
  }

  getStatus(): GatewayConnectionStatus {
    return this.status
  }

  connect(): void {
    if (!this.intentionallyClosed && (this.status === 'connecting' || this.status === 'connected')) {
      return
    }
    this.intentionallyClosed = false
    this.clearReconnectTimer()
    const seq = ++this.connectionSeq
    this.setStatus(this.readyCount > 0 ? 'reconnecting' : 'connecting')
    void this.openConnection(seq)
  }

  disconnect(): void {
    this.intentionallyClosed = true
    this.connectionSeq += 1
    this.readyCount = 0
    this.reconnectAttempt = 0
    this.clearReconnectTimer()
    this.clearHeartbeatTimers()

    const active = this.socket
    this.socket = null
    if (active) {
      active.onopen = null
      active.onmessage = null
      active.onerror = null
      active.onclose = null
      try {
        active.close(1000, 'client disconnect')
      } catch {
        // Ignore close errors on already-closed sockets.
      }
    }

    this.setStatus('disconnected')
  }

  reconnectNow(): void {
    if (this.intentionallyClosed) {
      return
    }
    this.clearReconnectTimer()
    this.clearHeartbeatTimers()
    const active = this.socket
    this.socket = null
    if (active) {
      active.onopen = null
      active.onmessage = null
      active.onerror = null
      active.onclose = null
      try {
        active.close(1000, 'manual reconnect')
      } catch {
        // Ignore close errors.
      }
    }
    this.reconnectAttempt = 0
    const seq = ++this.connectionSeq
    this.setStatus('reconnecting')
    void this.openConnection(seq)
  }

  send(frame: GatewayOutboundFrame): boolean {
    if (!this.socket || this.socket.readyState !== WS_READY_STATE_OPEN) {
      return false
    }
    this.socket.send(JSON.stringify(frame))
    return true
  }

  onEvent(listener: (event: GatewayEvent) => void): () => void {
    this.eventListeners.add(listener)
    return () => {
      this.eventListeners.delete(listener)
    }
  }

  onResync(listener: () => void): () => void {
    this.resyncListeners.add(listener)
    return () => {
      this.resyncListeners.delete(listener)
    }
  }

  onStatusChange(listener: (status: GatewayConnectionStatus) => void): () => void {
    this.statusListeners.add(listener)
    return () => {
      this.statusListeners.delete(listener)
    }
  }

  private setStatus(next: GatewayConnectionStatus): void {
    if (this.status === next) {
      return
    }
    this.status = next
    for (const listener of this.statusListeners) {
      listener(next)
    }
  }

  private async openConnection(seq: number): Promise<void> {
    try {
      const { ticket } = await this.fetchTicket()
      if (this.intentionallyClosed || seq !== this.connectionSeq) {
        return
      }

      const sep = this.wsUrl.includes('?') ? '&' : '?'
      const url = `${this.wsUrl}${sep}ticket=${encodeURIComponent(ticket)}`
      const ws = this.createSocket(url)
      this.socket = ws

      ws.onmessage = (ev) => {
        if (seq !== this.connectionSeq || this.socket !== ws) {
          return
        }
        if (typeof ev.data !== 'string') {
          return
        }
        const frame = parseGatewayFrame(ev.data)
        if (!frame) {
          this.log.warn('Ignoring invalid gateway WebSocket frame')
          return
        }
        this.handleFrame(ws, frame)
      }

      ws.onerror = () => {
        // Browsers dispatch onclose right after onerror; let onclose handle backoff scheduling.
      }

      ws.onclose = () => {
        if (seq !== this.connectionSeq) {
          return
        }
        if (this.socket === ws) {
          this.socket = null
        }
        this.clearHeartbeatTimers()
        if (!this.intentionallyClosed) {
          this.scheduleReconnect()
        }
      }
    } catch (err) {
      if (this.intentionallyClosed || seq !== this.connectionSeq) {
        return
      }
      if (err instanceof ApiError && err.status === 401 && this.hasAuthSession && !this.hasAuthSession()) {
        this.disconnect()
        return
      }
      this.scheduleReconnect()
    }
  }

  private handleFrame(ws: GatewaySocketLike, frame: GatewayEvent): void {
    if (frame.t === 'READY') {
      this.reconnectAttempt = 0
      this.readyCount += 1
      this.setStatus('connected')
      this.startHeartbeat(ws, frame.d.heartbeatIntervalMs)
      for (const listener of this.eventListeners) {
        listener(frame)
      }
      if (this.readyCount >= 2) {
        for (const listener of this.resyncListeners) {
          listener()
        }
      }
      return
    }

    if (frame.t === 'HEARTBEAT_ACK') {
      this.clearAckTimeout()
      for (const listener of this.eventListeners) {
        listener(frame)
      }
      return
    }

    for (const listener of this.eventListeners) {
      listener(frame)
    }
  }

  private startHeartbeat(ws: GatewaySocketLike, intervalMs: number): void {
    this.clearHeartbeatTimers()
    const safeIntervalMs = Math.max(50, intervalMs)

    this.heartbeatTimer = setInterval(() => {
      if (this.intentionallyClosed || this.socket !== ws || ws.readyState !== WS_READY_STATE_OPEN) {
        return
      }
      ws.send(JSON.stringify({ t: 'HEARTBEAT' }))
      this.armAckTimeout(ws)
    }, safeIntervalMs)
  }

  private armAckTimeout(ws: GatewaySocketLike): void {
    // Keep the earliest pending ACK deadline so an overdue ACK cannot be starved by later heartbeats.
    if (this.ackTimeoutTimer !== null) {
      return
    }
    this.ackTimeoutTimer = setTimeout(() => {
      this.ackTimeoutTimer = null
      if (this.intentionallyClosed || this.socket !== ws) {
        return
      }
      this.clearHeartbeatTimers()
      this.socket = null
      ws.onmessage = null
      ws.onerror = null
      ws.onclose = null
      try {
        ws.close(WS_CLOSE_HEARTBEAT_TIMEOUT, 'heartbeat timeout')
      } catch {
        // Ignore close errors.
      }
      this.scheduleReconnect()
    }, this.heartbeatAckTimeoutMs)
  }

  private scheduleReconnect(): void {
    if (this.intentionallyClosed || this.reconnectTimer !== null) {
      return
    }
    this.setStatus('reconnecting')
    const delayMs = computeReconnectDelayMs(this.reconnectAttempt, {
      baseDelayMs: this.baseReconnectDelayMs,
      maxDelayMs: this.maxReconnectDelayMs,
      random: this.random,
    })
    this.reconnectAttempt += 1

    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null
      if (this.intentionallyClosed) {
        return
      }
      const seq = ++this.connectionSeq
      void this.openConnection(seq)
    }, delayMs)
  }

  private clearHeartbeatTimers(): void {
    if (this.heartbeatTimer !== null) {
      clearInterval(this.heartbeatTimer)
      this.heartbeatTimer = null
    }
    this.clearAckTimeout()
  }

  private clearAckTimeout(): void {
    if (this.ackTimeoutTimer !== null) {
      clearTimeout(this.ackTimeoutTimer)
      this.ackTimeoutTimer = null
    }
  }

  private clearReconnectTimer(): void {
    if (this.reconnectTimer !== null) {
      clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }
  }
}

export const gatewayClient = new GatewayClient({
  wsUrl: DEFAULT_WS_BASE_URL,
  fetchTicket: () => wsApi.createTicket(),
})
