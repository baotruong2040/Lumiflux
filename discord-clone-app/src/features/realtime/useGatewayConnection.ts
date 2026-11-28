import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useSyncExternalStore } from 'react'
import type { User } from '../../core/api'
import { gatewayClient } from '../../core/gateway'
import type { GatewayConnectionStatus } from '../../core/gateway'
import { applyGatewayEventToCache, handleGatewayResync } from './realtimeHandlers'

function subscribeGatewayStatus(onStoreChange: () => void): () => void {
  return gatewayClient.onStatusChange(() => {
    onStoreChange()
  })
}

function getGatewayStatusSnapshot(): GatewayConnectionStatus {
  return gatewayClient.getStatus()
}

export function useGatewayConnection(user: User | null): GatewayConnectionStatus {
  const queryClient = useQueryClient()
  const status = useSyncExternalStore(subscribeGatewayStatus, getGatewayStatusSnapshot)
  const userId = user?.id ?? null

  useEffect(() => {
    if (!userId) {
      gatewayClient.disconnect()
      return
    }

    const offEvent = gatewayClient.onEvent((event) => {
      applyGatewayEventToCache(queryClient, event, userId)
    })
    const offResync = gatewayClient.onResync(() => {
      handleGatewayResync(queryClient)
    })

    const handleOnline = () => {
      gatewayClient.reconnectNow()
    }
    window.addEventListener('online', handleOnline)

    gatewayClient.connect()

    return () => {
      window.removeEventListener('online', handleOnline)
      offEvent()
      offResync()
      gatewayClient.disconnect()
    }
  }, [queryClient, userId])

  return userId ? status : 'disconnected'
}
