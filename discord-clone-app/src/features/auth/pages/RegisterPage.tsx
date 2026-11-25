import { useState } from 'react'
import type { FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { formatAuthErrorMessage, registerSchema } from '../schemas/authSchemas'
import type { RegisterFormValues } from '../schemas/authSchemas'
import { useAuthStore } from '../store/authStore'
import './AuthPages.css'

type RegisterField = keyof RegisterFormValues

export function RegisterPage() {
  const navigate = useNavigate()
  const registerAction = useAuthStore((s) => s.register)

  const [username, setUsername] = useState('')
  const [email, setEmail] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [password, setPassword] = useState('')
  const [fieldErrors, setFieldErrors] = useState<Partial<Record<RegisterField, string>>>({})
  const [serverError, setServerError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setServerError(null)

    const parsed = registerSchema.safeParse({ username, email, displayName, password })
    if (!parsed.success) {
      const nextErrors: Partial<Record<RegisterField, string>> = {}
      for (const issue of parsed.error.issues) {
        const key = issue.path[0] as RegisterField | undefined
        if (key && !nextErrors[key]) {
          nextErrors[key] = issue.message
        }
      }
      setFieldErrors(nextErrors)
      return
    }

    setFieldErrors({})
    setSubmitting(true)
    try {
      await registerAction(parsed.data)
      navigate('/app', { replace: true })
    } catch (err) {
      setServerError(formatAuthErrorMessage(err))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <main className="auth-page">
      <section className="auth-card" aria-labelledby="register-heading">
        <header className="auth-header">
          <span className="auth-badge">Lumiflux</span>
          <h1 id="register-heading" className="auth-title">
            Tạo tài khoản mới
          </h1>
          <p className="auth-subtitle">Tham gia trò chuyện thời gian thực trên Web &amp; Desktop.</p>
        </header>

        <form className="auth-form" onSubmit={handleSubmit} noValidate>
          {serverError && (
            <div className="auth-alert" role="alert">
              {serverError}
            </div>
          )}

          <div className="auth-field">
            <label className="auth-label" htmlFor="reg-username">
              Username
            </label>
            <input
              id="reg-username"
              name="username"
              type="text"
              autoComplete="username"
              className={`auth-input${fieldErrors.username ? ' auth-input-invalid' : ''}`}
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder="truong_dev"
              disabled={submitting}
              required
            />
            {fieldErrors.username && <p className="auth-field-error">{fieldErrors.username}</p>}
          </div>

          <div className="auth-field">
            <label className="auth-label" htmlFor="reg-email">
              Email
            </label>
            <input
              id="reg-email"
              name="email"
              type="email"
              autoComplete="email"
              className={`auth-input${fieldErrors.email ? ' auth-input-invalid' : ''}`}
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="truong@example.com"
              disabled={submitting}
              required
            />
            {fieldErrors.email && <p className="auth-field-error">{fieldErrors.email}</p>}
          </div>

          <div className="auth-field">
            <label className="auth-label" htmlFor="reg-display-name">
              Tên hiển thị
            </label>
            <input
              id="reg-display-name"
              name="displayName"
              type="text"
              autoComplete="name"
              className={`auth-input${fieldErrors.displayName ? ' auth-input-invalid' : ''}`}
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              placeholder="Truong"
              disabled={submitting}
              required
            />
            {fieldErrors.displayName && (
              <p className="auth-field-error">{fieldErrors.displayName}</p>
            )}
          </div>

          <div className="auth-field">
            <label className="auth-label" htmlFor="reg-password">
              Mật khẩu
            </label>
            <input
              id="reg-password"
              name="password"
              type="password"
              autoComplete="new-password"
              className={`auth-input${fieldErrors.password ? ' auth-input-invalid' : ''}`}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="Tối thiểu 8 ký tự"
              disabled={submitting}
              required
            />
            {fieldErrors.password && <p className="auth-field-error">{fieldErrors.password}</p>}
          </div>

          <button type="submit" className="auth-submit" disabled={submitting}>
            {submitting ? 'Đang tạo tài khoản...' : 'Đăng ký'}
          </button>
        </form>

        <footer className="auth-footer">
          Đã có tài khoản? <Link to="/login">Đăng nhập</Link>
        </footer>
      </section>
    </main>
  )
}
