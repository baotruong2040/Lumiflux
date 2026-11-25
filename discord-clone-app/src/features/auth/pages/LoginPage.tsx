import { useState } from 'react'
import type { FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { formatAuthErrorMessage, loginSchema } from '../schemas/authSchemas'
import { useAuthStore } from '../store/authStore'
import './AuthPages.css'

export function LoginPage() {
  const navigate = useNavigate()
  const loginAction = useAuthStore((s) => s.login)

  const [login, setLogin] = useState('')
  const [password, setPassword] = useState('')
  const [fieldErrors, setFieldErrors] = useState<Partial<Record<'login' | 'password', string>>>({})
  const [serverError, setServerError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setServerError(null)

    const parsed = loginSchema.safeParse({ login, password })
    if (!parsed.success) {
      const nextErrors: Partial<Record<'login' | 'password', string>> = {}
      for (const issue of parsed.error.issues) {
        const key = issue.path[0]
        if ((key === 'login' || key === 'password') && !nextErrors[key]) {
          nextErrors[key] = issue.message
        }
      }
      setFieldErrors(nextErrors)
      return
    }

    setFieldErrors({})
    setSubmitting(true)
    try {
      await loginAction(parsed.data)
      navigate('/app', { replace: true })
    } catch (err) {
      setServerError(formatAuthErrorMessage(err))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <main className="auth-page">
      <section className="auth-card" aria-labelledby="login-heading">
        <header className="auth-header">
          <span className="auth-badge">Lumiflux</span>
          <h1 id="login-heading" className="auth-title">
            Chào mừng trở lại
          </h1>
          <p className="auth-subtitle">Đăng nhập để tiếp tục trò chuyện cùng máy chủ của bạn.</p>
        </header>

        <form className="auth-form" onSubmit={handleSubmit} noValidate>
          {serverError && (
            <div className="auth-alert" role="alert">
              {serverError}
            </div>
          )}

          <div className="auth-field">
            <label className="auth-label" htmlFor="login-identifier">
              Username hoặc Email
            </label>
            <input
              id="login-identifier"
              name="login"
              type="text"
              autoComplete="username"
              className={`auth-input${fieldErrors.login ? ' auth-input-invalid' : ''}`}
              value={login}
              onChange={(e) => setLogin(e.target.value)}
              placeholder="truong_dev hoặc truong@example.com"
              disabled={submitting}
              required
            />
            {fieldErrors.login && <p className="auth-field-error">{fieldErrors.login}</p>}
          </div>

          <div className="auth-field">
            <label className="auth-label" htmlFor="login-password">
              Mật khẩu
            </label>
            <input
              id="login-password"
              name="password"
              type="password"
              autoComplete="current-password"
              className={`auth-input${fieldErrors.password ? ' auth-input-invalid' : ''}`}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="Nhập mật khẩu"
              disabled={submitting}
              required
            />
            {fieldErrors.password && <p className="auth-field-error">{fieldErrors.password}</p>}
          </div>

          <button type="submit" className="auth-submit" disabled={submitting}>
            {submitting ? 'Đang đăng nhập...' : 'Đăng nhập'}
          </button>
        </form>

        <footer className="auth-footer">
          Chưa có tài khoản? <Link to="/register">Đăng ký ngay</Link>
        </footer>
      </section>
    </main>
  )
}
