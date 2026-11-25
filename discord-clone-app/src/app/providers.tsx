import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useEffect } from 'react'
import type { ReactNode } from 'react'
import { useAuthStore } from '../features/auth/store/authStore'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
})

export function AppProviders({ children }: { children: ReactNode }) {
  const hydrate = useAuthStore((s) => s.hydrate)

  useEffect(() => {
    void hydrate()
  }, [hydrate])

  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
}
