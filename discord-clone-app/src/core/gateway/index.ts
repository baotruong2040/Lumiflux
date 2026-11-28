export {
  computeReconnectDelayMs,
  DEFAULT_BASE_RECONNECT_DELAY_MS,
  DEFAULT_HEARTBEAT_ACK_TIMEOUT_MS,
  DEFAULT_MAX_RECONNECT_DELAY_MS,
  deriveWsUrl,
  GatewayClient,
  gatewayClient,
  WS_CLOSE_HEARTBEAT_TIMEOUT,
} from './GatewayClient'
export type {
  GatewayClientOptions,
  GatewayConnectionStatus,
  GatewaySocketLike,
} from './GatewayClient'
export {
  gatewayEventSchema,
  parseGatewayFrame,
} from './events'
export type { GatewayEvent, GatewayOutboundFrame } from './events'
