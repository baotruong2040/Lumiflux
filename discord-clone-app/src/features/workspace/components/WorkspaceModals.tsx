import { useState } from 'react'
import type { FormEvent } from 'react'
import type { Channel, Guild, Invite } from '../../../core/api'
import {
  channelFormSchema,
  createGuildSchema,
  editGuildSchema,
  extractInviteCode,
  formatWorkspaceError,
  joinGuildSchema,
} from '../utils/workspaceHelpers'
import { CheckIcon, CopyIcon } from './Icons'
import { Modal } from './Modal'

export interface CreateServerModalProps {
  open: boolean
  isPending: boolean
  onClose: () => void
  onSubmit: (payload: { name: string; iconUrl: string | null }) => Promise<void>
}

function CreateServerForm({
  isPending,
  onClose,
  onSubmit,
}: Omit<CreateServerModalProps, 'open'>) {
  const [name, setName] = useState('')
  const [iconUrl, setIconUrl] = useState('')
  const [error, setError] = useState<string | null>(null)

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setError(null)

    const parsed = createGuildSchema.safeParse({ name, iconUrl })
    if (!parsed.success) {
      setError(parsed.error.issues[0]?.message ?? 'Dữ liệu chưa hợp lệ')
      return
    }

    try {
      await onSubmit({
        name: parsed.data.name,
        iconUrl: parsed.data.iconUrl ? parsed.data.iconUrl : null,
      })
      onClose()
    } catch (err) {
      setError(formatWorkspaceError(err))
    }
  }

  return (
    <form className="ws-form" onSubmit={handleSubmit} noValidate>
      {error && (
        <div className="ws-alert ws-alert-danger" role="alert">
          {error}
        </div>
      )}

      <label className="ws-field">
        <span className="ws-label">Tên server</span>
        <input
          type="text"
          className="ws-input"
          placeholder="Ví dụ: Lumiflux Engineering"
          value={name}
          onChange={(e) => setName(e.target.value)}
          maxLength={100}
          autoFocus
        />
      </label>

      <label className="ws-field">
        <span className="ws-label">Icon URL (tùy chọn)</span>
        <input
          type="url"
          className="ws-input"
          placeholder="https://example.com/icon.png"
          value={iconUrl}
          onChange={(e) => setIconUrl(e.target.value)}
        />
      </label>

      <div className="ws-modal-footer">
        <button type="button" className="ws-btn ws-btn-ghost" onClick={onClose} disabled={isPending}>
          Hủy
        </button>
        <button type="submit" className="ws-btn ws-btn-primary" disabled={isPending}>
          {isPending ? 'Đang tạo...' : 'Tạo server'}
        </button>
      </div>
    </form>
  )
}

export function CreateServerModal({ open, isPending, onClose, onSubmit }: CreateServerModalProps) {
  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Tạo server mới"
      subtitle="Server của bạn sẽ tự động có sẵn kênh #general và gán bạn làm Owner."
    >
      {open && <CreateServerForm isPending={isPending} onClose={onClose} onSubmit={onSubmit} />}
    </Modal>
  )
}

export interface EditServerModalProps {
  open: boolean
  guild: Guild | null
  isPending: boolean
  onClose: () => void
  onSubmit: (guildId: string, payload: { name: string; iconUrl: string | null }) => Promise<void>
}

function EditServerForm({
  guild,
  isPending,
  onClose,
  onSubmit,
}: {
  guild: Guild
  isPending: boolean
  onClose: () => void
  onSubmit: (guildId: string, payload: { name: string; iconUrl: string | null }) => Promise<void>
}) {
  const [name, setName] = useState(guild.name)
  const [iconUrl, setIconUrl] = useState(guild.iconUrl ?? '')
  const [error, setError] = useState<string | null>(null)

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setError(null)

    const parsed = editGuildSchema.safeParse({ name, iconUrl })
    if (!parsed.success) {
      setError(parsed.error.issues[0]?.message ?? 'Dữ liệu chưa hợp lệ')
      return
    }

    try {
      await onSubmit(guild.id, {
        name: parsed.data.name,
        iconUrl: parsed.data.iconUrl ? parsed.data.iconUrl : null,
      })
      onClose()
    } catch (err) {
      setError(formatWorkspaceError(err))
    }
  }

  return (
    <form className="ws-form" onSubmit={handleSubmit} noValidate>
      {error && (
        <div className="ws-alert ws-alert-danger" role="alert">
          {error}
        </div>
      )}

      <label className="ws-field">
        <span className="ws-label">Tên server</span>
        <input
          type="text"
          className="ws-input"
          value={name}
          onChange={(e) => setName(e.target.value)}
          maxLength={100}
          autoFocus
        />
      </label>

      <label className="ws-field">
        <span className="ws-label">Icon URL (để trống để xóa icon)</span>
        <input
          type="url"
          className="ws-input"
          placeholder="https://example.com/icon.png"
          value={iconUrl}
          onChange={(e) => setIconUrl(e.target.value)}
        />
      </label>

      <div className="ws-modal-footer">
        <button type="button" className="ws-btn ws-btn-ghost" onClick={onClose} disabled={isPending}>
          Hủy
        </button>
        <button type="submit" className="ws-btn ws-btn-primary" disabled={isPending}>
          {isPending ? 'Đang lưu...' : 'Lưu thay đổi'}
        </button>
      </div>
    </form>
  )
}

export function EditServerModal({ open, guild, isPending, onClose, onSubmit }: EditServerModalProps) {
  return (
    <Modal open={open} onClose={onClose} title="Chỉnh sửa server" subtitle="Cập nhật tên hoặc ảnh đại diện của server.">
      {open && guild && (
        <EditServerForm
          key={guild.id}
          guild={guild}
          isPending={isPending}
          onClose={onClose}
          onSubmit={onSubmit}
        />
      )}
    </Modal>
  )
}

export interface JoinServerModalProps {
  open: boolean
  isPending: boolean
  onClose: () => void
  onSubmit: (code: string) => Promise<void>
}

function JoinServerForm({
  isPending,
  onClose,
  onSubmit,
}: Omit<JoinServerModalProps, 'open'>) {
  const [rawCode, setRawCode] = useState('')
  const [error, setError] = useState<string | null>(null)

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setError(null)

    const code = extractInviteCode(rawCode)
    const parsed = joinGuildSchema.safeParse({ code })
    if (!parsed.success) {
      setError(parsed.error.issues[0]?.message ?? 'Vui lòng nhập mã mời')
      return
    }

    try {
      await onSubmit(parsed.data.code)
      onClose()
    } catch (err) {
      setError(formatWorkspaceError(err))
    }
  }

  return (
    <form className="ws-form" onSubmit={handleSubmit} noValidate>
      {error && (
        <div className="ws-alert ws-alert-danger" role="alert">
          {error}
        </div>
      )}

      <label className="ws-field">
        <span className="ws-label">Mã mời</span>
        <input
          type="text"
          className="ws-input ws-input-mono"
          placeholder="Ví dụ: aB3k9ZpQ"
          value={rawCode}
          onChange={(e) => setRawCode(e.target.value)}
          autoFocus
        />
      </label>

      <div className="ws-modal-footer">
        <button type="button" className="ws-btn ws-btn-ghost" onClick={onClose} disabled={isPending}>
          Hủy
        </button>
        <button type="submit" className="ws-btn ws-btn-success" disabled={isPending}>
          {isPending ? 'Đang tham gia...' : 'Tham gia server'}
        </button>
      </div>
    </form>
  )
}

export function JoinServerModal({ open, isPending, onClose, onSubmit }: JoinServerModalProps) {
  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Tham gia server"
      subtitle="Nhập mã mời được thành viên server chia sẻ để tham gia ngay."
    >
      {open && <JoinServerForm isPending={isPending} onClose={onClose} onSubmit={onSubmit} />}
    </Modal>
  )
}

export interface CreateInviteModalProps {
  open: boolean
  guild: Guild | null
  isPending: boolean
  onClose: () => void
  onGenerate: (guildId: string, payload: { maxUses: number | null; expiresInHours: number | null }) => Promise<Invite>
}

function CreateInviteForm({
  guild,
  isPending,
  onGenerate,
}: {
  guild: Guild
  isPending: boolean
  onGenerate: (guildId: string, payload: { maxUses: number | null; expiresInHours: number | null }) => Promise<Invite>
}) {
  const [maxUsesStr, setMaxUsesStr] = useState('')
  const [expiresHoursStr, setExpiresHoursStr] = useState('168')
  const [invite, setInvite] = useState<Invite | null>(null)
  const [copied, setCopied] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function handleGenerate(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setError(null)
    setCopied(false)

    const trimmedUses = maxUsesStr.trim()
    let maxUses: number | null = null
    if (trimmedUses !== '') {
      const parsedUses = Number.parseInt(trimmedUses, 10)
      if (!Number.isInteger(parsedUses) || parsedUses < 1) {
        setError('Số lượt tối đa phải là số nguyên lớn hơn 0 (hoặc để trống).')
        return
      }
      maxUses = parsedUses
    }

    const expiresInHours = expiresHoursStr === 'never' ? null : Number.parseInt(expiresHoursStr, 10)

    try {
      const created = await onGenerate(guild.id, { maxUses, expiresInHours })
      setInvite(created)
    } catch (err) {
      setError(formatWorkspaceError(err))
    }
  }

  async function handleCopy() {
    if (!invite) return
    try {
      await navigator.clipboard.writeText(invite.code)
      setCopied(true)
    } catch {
      setCopied(false)
    }
  }

  return (
    <form className="ws-form" onSubmit={handleGenerate} noValidate>
      {error && (
        <div className="ws-alert ws-alert-danger" role="alert">
          {error}
        </div>
      )}

      <div className="ws-invite-options">
        <label className="ws-field">
          <span className="ws-label">Hạn sử dụng</span>
          <select
            className="ws-input"
            value={expiresHoursStr}
            onChange={(e) => setExpiresHoursStr(e.target.value)}
          >
            <option value="1">1 giờ</option>
            <option value="24">24 giờ</option>
            <option value="168">7 ngày (mặc định)</option>
            <option value="never">Không giới hạn</option>
          </select>
        </label>

        <label className="ws-field">
          <span className="ws-label">Số lượt tối đa</span>
          <input
            type="number"
            min={1}
            className="ws-input"
            placeholder="Không giới hạn"
            value={maxUsesStr}
            onChange={(e) => setMaxUsesStr(e.target.value)}
          />
        </label>
      </div>

      <button type="submit" className="ws-btn ws-btn-primary ws-btn-block" disabled={isPending}>
        {isPending ? 'Đang tạo mã mời...' : invite ? 'Tạo mã mời mới' : 'Tạo mã mời'}
      </button>

      {invite && (
        <div className="ws-invite-result">
          <span className="ws-label">Mã mời đã sẵn sàng</span>
          <div className="ws-invite-copy-row">
            <input
              type="text"
              readOnly
              className="ws-input ws-input-mono"
              value={invite.code}
              aria-label="Mã mời server"
              onFocus={(e) => e.currentTarget.select()}
            />
            <button type="button" className="ws-btn ws-btn-success ws-btn-icon-text" onClick={handleCopy}>
              {copied ? <CheckIcon /> : <CopyIcon />}
              <span>{copied ? 'Đã chép' : 'Sao chép'}</span>
            </button>
          </div>
          <p className="ws-invite-meta">
            Đã dùng: {invite.uses}
            {invite.maxUses !== null ? ` / ${invite.maxUses}` : ' (không giới hạn lượt)'}
            {invite.expiresAt
              ? ` • Hết hạn lúc ${new Date(invite.expiresAt).toLocaleString('vi-VN')}`
              : ' • Không giới hạn thời gian'}
          </p>
        </div>
      )}
    </form>
  )
}

export function CreateInviteModal({ open, guild, isPending, onClose, onGenerate }: CreateInviteModalProps) {
  return (
    <Modal
      open={open}
      onClose={onClose}
      title={guild ? `Mời bạn bè vào ${guild.name}` : 'Tạo mã mời'}
      subtitle="Gửi mã mời này cho tài khoản khác để họ tham gia server."
    >
      {open && guild && (
        <CreateInviteForm key={guild.id} guild={guild} isPending={isPending} onGenerate={onGenerate} />
      )}
    </Modal>
  )
}

export interface ChannelFormModalProps {
  open: boolean
  mode: 'create' | 'edit'
  channel: Channel | null
  isPending: boolean
  onClose: () => void
  onSubmit: (payload: { name: string; topic: string | null }) => Promise<void>
}

function ChannelFormInner({
  mode,
  channel,
  isPending,
  onClose,
  onSubmit,
}: Omit<ChannelFormModalProps, 'open'>) {
  const [name, setName] = useState(mode === 'edit' && channel ? channel.name : '')
  const [topic, setTopic] = useState(mode === 'edit' && channel ? (channel.topic ?? '') : '')
  const [error, setError] = useState<string | null>(null)

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setError(null)

    const parsed = channelFormSchema.safeParse({ name, topic })
    if (!parsed.success) {
      setError(parsed.error.issues[0]?.message ?? 'Dữ liệu kênh chưa hợp lệ')
      return
    }

    try {
      await onSubmit({
        name: parsed.data.name,
        topic: parsed.data.topic ? parsed.data.topic : null,
      })
      onClose()
    } catch (err) {
      setError(formatWorkspaceError(err))
    }
  }

  return (
    <form className="ws-form" onSubmit={handleSubmit} noValidate>
      {error && (
        <div className="ws-alert ws-alert-danger" role="alert">
          {error}
        </div>
      )}

      <label className="ws-field">
        <span className="ws-label">Tên kênh</span>
        <input
          type="text"
          className="ws-input"
          placeholder="Ví dụ: backend-api"
          value={name}
          onChange={(e) => setName(e.target.value)}
          maxLength={100}
          autoFocus
        />
      </label>

      <label className="ws-field">
        <span className="ws-label">Chủ đề / mô tả (tùy chọn)</span>
        <textarea
          className="ws-input ws-textarea"
          rows={3}
          placeholder="Kênh thảo luận kiến trúc API và database..."
          value={topic}
          onChange={(e) => setTopic(e.target.value)}
          maxLength={255}
        />
      </label>

      <div className="ws-modal-footer">
        <button type="button" className="ws-btn ws-btn-ghost" onClick={onClose} disabled={isPending}>
          Hủy
        </button>
        <button type="submit" className="ws-btn ws-btn-primary" disabled={isPending}>
          {isPending ? 'Đang lưu...' : mode === 'create' ? 'Tạo kênh' : 'Lưu thay đổi'}
        </button>
      </div>
    </form>
  )
}

export function ChannelFormModal({ open, mode, channel, isPending, onClose, onSubmit }: ChannelFormModalProps) {
  return (
    <Modal
      open={open}
      onClose={onClose}
      title={mode === 'create' ? 'Tạo kênh văn bản' : `Chỉnh sửa #${channel?.name ?? 'kênh'}`}
      subtitle={
        mode === 'create'
          ? 'Tên kênh phải là duy nhất trong cùng server.'
          : 'Cập nhật tên hiển thị hoặc mô tả chủ đề của kênh.'
      }
    >
      {open && (
        <ChannelFormInner
          key={`${mode}-${channel?.id ?? 'new'}`}
          mode={mode}
          channel={channel}
          isPending={isPending}
          onClose={onClose}
          onSubmit={onSubmit}
        />
      )}
    </Modal>
  )
}

export interface ConfirmActionModalProps {
  open: boolean
  title: string
  description: string
  confirmLabel: string
  isPending: boolean
  onClose: () => void
  onConfirm: () => Promise<void>
}

function ConfirmActionInner({
  description,
  confirmLabel,
  isPending,
  onClose,
  onConfirm,
}: Omit<ConfirmActionModalProps, 'open' | 'title'>) {
  const [error, setError] = useState<string | null>(null)

  async function handleConfirm() {
    setError(null)
    try {
      await onConfirm()
      onClose()
    } catch (err) {
      setError(formatWorkspaceError(err))
    }
  }

  return (
    <div className="ws-form">
      {error && (
        <div className="ws-alert ws-alert-danger" role="alert">
          {error}
        </div>
      )}
      <p className="ws-confirm-text">{description}</p>
      <div className="ws-modal-footer">
        <button type="button" className="ws-btn ws-btn-ghost" onClick={onClose} disabled={isPending}>
          Hủy
        </button>
        <button type="button" className="ws-btn ws-btn-danger" onClick={handleConfirm} disabled={isPending}>
          {isPending ? 'Đang xử lý...' : confirmLabel}
        </button>
      </div>
    </div>
  )
}

export function ConfirmActionModal({
  open,
  title,
  description,
  confirmLabel,
  isPending,
  onClose,
  onConfirm,
}: ConfirmActionModalProps) {
  return (
    <Modal open={open} onClose={onClose} title={title}>
      {open && (
        <ConfirmActionInner
          key={title}
          description={description}
          confirmLabel={confirmLabel}
          isPending={isPending}
          onClose={onClose}
          onConfirm={onConfirm}
        />
      )}
    </Modal>
  )
}
