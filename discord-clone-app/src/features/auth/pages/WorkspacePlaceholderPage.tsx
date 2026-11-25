import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { authApi } from '../../../core/api'
import { isTauriRuntime } from '../../../core/platform'
import { formatAuthErrorMessage } from '../schemas/authSchemas'
import { useAuthStore } from '../store/authStore'
import './AuthPages.css'

export function WorkspacePlaceholderPage() {
  const navigate = useNavigate()
  const user = useAuthStore((s) => s.user)
  const setUser = useAuthStore((s) => s.setUser)
  const logout = useAuthStore((s) => s.logout)

  const [refreshing, setRefreshing] = useState(false)
  const [loggingOut, setLoggingOut] = useState(false)
  const [feedback, setFeedback] = useState<string | null>(null)

  async function handleRefreshProfile() {
    setRefreshing(true)
    setFeedback(null)
    try {
      const me = await authApi.getMe()
      setUser(me)
      setFeedback(`Đã đồng bộ hồ sơ @me lúc ${new Date().toLocaleTimeString('vi-VN')}`)
    } catch (err) {
      setFeedback(formatAuthErrorMessage(err))
    } finally {
      setRefreshing(false)
    }
  }

  async function handleLogout() {
    setLoggingOut(true)
    await logout()
    navigate('/login', { replace: true })
  }

  const platformLabel = isTauriRuntime() ? 'Tauri Desktop' : 'Web Browser'

  return (
    <main className="workspace-shell">
      <section className="workspace-card" aria-labelledby="workspace-heading">
        <header className="workspace-top">
          <h1 id="workspace-heading" className="workspace-title">
            Phiên đăng nhập Lumiflux
          </h1>
          <span className="workspace-platform">{platformLabel}</span>
        </header>

        {user && (
          <div className="workspace-profile">
            <div className="workspace-row">
              <span className="workspace-key">Tên hiển thị</span>
              <span className="workspace-val">{user.displayName}</span>
            </div>
            <div className="workspace-row">
              <span className="workspace-key">Username</span>
              <span className="workspace-val">@{user.username}</span>
            </div>
            <div className="workspace-row">
              <span className="workspace-key">Email</span>
              <span className="workspace-val">{user.email}</span>
            </div>
            <div className="workspace-row">
              <span className="workspace-key">User ID (UUIDv7)</span>
              <span className="workspace-val workspace-mono">{user.id}</span>
            </div>
          </div>
        )}

        {feedback && (
          <p className="workspace-feedback" role="status">
            {feedback}
          </p>
        )}

        <div className="workspace-actions">
          <button
            type="button"
            className="btn-secondary"
            onClick={handleRefreshProfile}
            disabled={refreshing || loggingOut}
          >
            {refreshing ? 'Đang kiểm tra...' : 'Làm mới hồ sơ (@me)'}
          </button>

          <button
            type="button"
            className="btn-danger"
            onClick={handleLogout}
            disabled={loggingOut}
          >
            {loggingOut ? 'Đang đăng xuất...' : 'Đăng xuất'}
          </button>
        </div>
      </section>
    </main>
  )
}
