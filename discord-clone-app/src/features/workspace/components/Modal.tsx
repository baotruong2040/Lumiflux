import { useEffect } from 'react'
import type { ReactNode } from 'react'
import { CloseIcon } from './Icons'

export interface ModalProps {
  open: boolean
  title: string
  subtitle?: string
  onClose: () => void
  children: ReactNode
}

export function Modal({ open, title, subtitle, onClose, children }: ModalProps) {
  useEffect(() => {
    if (!open) return

    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        onClose()
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [open, onClose])

  if (!open) {
    return null
  }

  return (
    <div
      className="ws-modal-backdrop"
      role="presentation"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) {
          onClose()
        }
      }}
    >
      <section className="ws-modal" role="dialog" aria-modal="true" aria-labelledby="ws-modal-title">
        <header className="ws-modal-header">
          <div>
            <h2 id="ws-modal-title" className="ws-modal-title">
              {title}
            </h2>
            {subtitle && <p className="ws-modal-subtitle">{subtitle}</p>}
          </div>
          <button
            type="button"
            className="ws-modal-close"
            onClick={onClose}
            aria-label="Đóng hộp thoại"
          >
            <CloseIcon />
          </button>
        </header>
        <div className="ws-modal-body">{children}</div>
      </section>
    </div>
  )
}
