import type { ReactNode } from 'react'
import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { useAuthStore } from '../store/authStore'

function AuthHydrationScreen() {
  return (
    <div className="auth-loading-shell" role="status" aria-live="polite">
      <div className="auth-loading-card">
        <div className="auth-spinner" aria-hidden="true" />
        <p className="auth-loading-text">Đang khôi phục phiên đăng nhập...</p>
      </div>
    </div>
  )
}

export function ProtectedRoute({ children }: { children?: ReactNode }) {
  const status = useAuthStore((s) => s.status)
  const location = useLocation()

  if (status === 'idle' || status === 'hydrating') {
    return <AuthHydrationScreen />
  }

  if (status !== 'authenticated') {
    return <Navigate to="/login" replace state={{ from: location }} />
  }

  return children ? <>{children}</> : <Outlet />
}

export function PublicOnlyRoute({ children }: { children?: ReactNode }) {
  const status = useAuthStore((s) => s.status)

  if (status === 'idle' || status === 'hydrating') {
    return <AuthHydrationScreen />
  }

  if (status === 'authenticated') {
    return <Navigate to="/app" replace />
  }

  return children ? <>{children}</> : <Outlet />
}
