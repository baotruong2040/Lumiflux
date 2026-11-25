import { useLayoutEffect, useRef, useState } from 'react'
import type { CompositionEvent, KeyboardEvent } from 'react'
import type { Channel, GuildRole, User } from '../../../core/api'
import { EditIcon, HashIcon, TrashIcon } from '../../workspace/components/Icons'
import { ConfirmActionModal } from '../../workspace/components/WorkspaceModals'
import { getInitials } from '../../workspace/utils/workspaceHelpers'
import type { ChatMessage } from '../types'
import { formatChatError, formatMessageTimestamp } from '../utils/messageHelpers'

export interface MessageListProps {
  guildName: string
  channel: Channel
  currentUser: User | null
  role: GuildRole | null
  messages: ChatMessage[]
  isLoading: boolean
  isError: boolean
  error: unknown
  hasNextPage: boolean
  isFetchingNextPage: boolean
  onFetchNextPage: () => Promise<unknown>
  onRetryLoad: () => Promise<unknown>
  onRetrySend: (msg: ChatMessage) => Promise<void>
  onDismissFailed: (idOrNonce: string) => void
  onEditMessage: (messageId: string, content: string) => Promise<unknown>
  onDeleteMessage: (messageId: string) => Promise<void>
  isDeleting: boolean
  onOpenEditChannel: (channel: Channel) => void
}

export function MessageList({
  guildName,
  channel,
  currentUser,
  role,
  messages,
  isLoading,
  isError,
  error,
  hasNextPage,
  isFetchingNextPage,
  onFetchNextPage,
  onRetryLoad,
  onRetrySend,
  onDismissFailed,
  onEditMessage,
  onDeleteMessage,
  isDeleting,
  onOpenEditChannel,
}: MessageListProps) {
  const scrollContainerRef = useRef<HTMLDivElement | null>(null)
  const scrollSnapshotRef = useRef<{ scrollHeight: number; scrollTop: number } | null>(null)
  const initialScrolledRef = useRef(false)
  const wasAtBottomRef = useRef(true)
  const lastTailIdRef = useRef<string | null>(null)
  const fetchingLockRef = useRef(false)
  const editComposingRef = useRef(false)

  const [editingId, setEditingId] = useState<string | null>(null)
  const [editDraft, setEditDraft] = useState('')
  const [editError, setEditError] = useState<string | null>(null)
  const [savingEdit, setSavingEdit] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<ChatMessage | null>(null)

  const canManageChannels = role === 'owner' || role === 'admin'

  useLayoutEffect(() => {
    const el = scrollContainerRef.current
    if (!el) return

    if (scrollSnapshotRef.current !== null) {
      const snap = scrollSnapshotRef.current
      el.scrollTop = el.scrollHeight - snap.scrollHeight + snap.scrollTop
      scrollSnapshotRef.current = null
      return
    }

    if (!initialScrolledRef.current && messages.length > 0) {
      el.scrollTop = el.scrollHeight
      initialScrolledRef.current = true
      lastTailIdRef.current = messages[messages.length - 1]?.id ?? null
      return
    }

    const latest = messages[messages.length - 1]
    const latestId = latest?.id ?? null
    if (latestId && latestId !== lastTailIdRef.current) {
      if (wasAtBottomRef.current || (currentUser && latest.author.id === currentUser.id)) {
        el.scrollTop = el.scrollHeight
      }
      lastTailIdRef.current = latestId
    }
  }, [messages, currentUser])

  function triggerLoadOlder() {
    const el = scrollContainerRef.current
    if (!el || !hasNextPage || isFetchingNextPage || fetchingLockRef.current) {
      return
    }
    scrollSnapshotRef.current = {
      scrollHeight: el.scrollHeight,
      scrollTop: el.scrollTop,
    }
    fetchingLockRef.current = true
    void onFetchNextPage().finally(() => {
      fetchingLockRef.current = false
    })
  }

  function handleScroll() {
    const el = scrollContainerRef.current
    if (!el) return

    wasAtBottomRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 64
    if (el.scrollTop <= 80) {
      triggerLoadOlder()
    }
  }

  function startEditing(msg: ChatMessage) {
    setEditingId(msg.id)
    setEditDraft(msg.content)
    setEditError(null)
  }

  function cancelEditing() {
    setEditingId(null)
    setEditDraft('')
    setEditError(null)
  }

  async function saveEdit(msg: ChatMessage) {
    const trimmed = editDraft.trim()
    if (!trimmed) {
      setEditError('Nội dung tin nhắn không được để trống.')
      return
    }
    if ([...trimmed].length > 2000) {
      setEditError('Tin nhắn tối đa 2.000 ký tự.')
      return
    }
    if (trimmed === msg.content) {
      cancelEditing()
      return
    }

    setSavingEdit(true)
    setEditError(null)
    try {
      await onEditMessage(msg.id, trimmed)
      cancelEditing()
    } catch (err) {
      setEditError(formatChatError(err))
    } finally {
      setSavingEdit(false)
    }
  }

  function handleEditCompositionStart(_e: CompositionEvent<HTMLTextAreaElement>) {
    editComposingRef.current = true
  }

  function handleEditCompositionEnd(_e: CompositionEvent<HTMLTextAreaElement>) {
    editComposingRef.current = false
  }

  function handleEditKeyDown(e: KeyboardEvent<HTMLTextAreaElement>, msg: ChatMessage) {
    if (e.key === 'Escape') {
      e.preventDefault()
      cancelEditing()
      return
    }
    if (e.key === 'Enter' && !e.shiftKey) {
      if (editComposingRef.current || e.nativeEvent.isComposing || e.keyCode === 229) {
        return
      }
      e.preventDefault()
      void saveEdit(msg)
    }
  }

  return (
    <>
      <div
        ref={scrollContainerRef}
        className="chat-list-scroll"
        onScroll={handleScroll}
        aria-label={`Lịch sử tin nhắn kênh #${channel.name}`}
      >
        {hasNextPage && (
          <div className="chat-load-more-bar">
            <button
              type="button"
              className="ws-btn ws-btn-ghost chat-load-more-btn"
              onClick={triggerLoadOlder}
              disabled={isFetchingNextPage}
            >
              {isFetchingNextPage ? 'Đang tải tin nhắn cũ...' : 'Tải thêm tin nhắn cũ'}
            </button>
          </div>
        )}

        {!hasNextPage && !isLoading && (
          <div className="ws-channel-hero chat-channel-hero">
            <div className="ws-channel-hero-icon">
              <HashIcon width={32} height={32} />
            </div>
            <h2 className="ws-channel-hero-title">Chào mừng đến với #{channel.name}!</h2>
            <p className="ws-channel-hero-desc">
              {channel.topic
                ? channel.topic
                : `Đây là điểm khởi đầu của kênh #${channel.name} thuộc server ${guildName}.`}
            </p>
            {canManageChannels && (
              <button
                type="button"
                className="ws-inline-link-btn"
                onClick={() => onOpenEditChannel(channel)}
              >
                <EditIcon />
                <span>Chỉnh sửa kênh</span>
              </button>
            )}
          </div>
        )}

        {isLoading ? (
          <div className="chat-state-banner">Đang tải lịch sử tin nhắn...</div>
        ) : isError ? (
          <div className="chat-state-banner chat-state-error" role="alert">
            <span>{formatChatError(error)}</span>
            <button type="button" className="ws-btn ws-btn-primary" onClick={() => void onRetryLoad()}>
              Thử lại
            </button>
          </div>
        ) : messages.length === 0 ? (
          <div className="chat-empty-hint">Chưa có tin nhắn nào trong #{channel.name}. Hãy là người đầu tiên trò chuyện!</div>
        ) : (
          <ol className="chat-message-list">
            {messages.map((msg) => {
              const isAuthor = currentUser?.id === msg.author.id
              const isOptimistic = Boolean(msg.optimisticStatus)
              const canEdit = isAuthor && !isOptimistic
              const canDelete = (isAuthor || canManageChannels) && !isOptimistic
              const isEditingThis = editingId === msg.id

              return (
                <li
                  key={msg.id}
                  className={`chat-message-item ${
                    msg.optimisticStatus === 'sending'
                      ? 'chat-message-pending'
                      : msg.optimisticStatus === 'failed'
                        ? 'chat-message-failed'
                        : ''
                  }`}
                >
                  <div className="chat-message-avatar" aria-hidden="true">
                    {msg.author.avatarUrl ? (
                      <img
                        src={msg.author.avatarUrl}
                        alt={msg.author.displayName}
                        className="ws-user-avatar-img"
                      />
                    ) : (
                      <span>{getInitials(msg.author.displayName || msg.author.username)}</span>
                    )}
                  </div>

                  <div className="chat-message-body">
                    <div className="chat-message-meta">
                      <span className="chat-message-author">{msg.author.displayName}</span>
                      <span className="chat-message-username">@{msg.author.username}</span>
                      <time className="chat-message-time" dateTime={msg.createdAt}>
                        {formatMessageTimestamp(msg.createdAt)}
                      </time>
                      {msg.editedAt && (
                        <span className="chat-edited-tag">(đã chỉnh sửa)</span>
                      )}
                      {msg.optimisticStatus === 'sending' && (
                        <span className="chat-sending-tag">Đang gửi...</span>
                      )}
                    </div>

                    {isEditingThis ? (
                      <div className="chat-edit-box">
                        {editError && (
                          <div className="chat-composer-error" role="alert">
                            {editError}
                          </div>
                        )}
                        <textarea
                          className="ws-input ws-textarea chat-edit-input"
                          rows={2}
                          value={editDraft}
                          onChange={(e) => setEditDraft(e.target.value)}
                          onCompositionStart={handleEditCompositionStart}
                          onCompositionEnd={handleEditCompositionEnd}
                          onKeyDown={(e) => handleEditKeyDown(e, msg)}
                          aria-label="Chỉnh sửa nội dung tin nhắn"
                          autoFocus
                        />
                        <div className="chat-edit-actions">
                          <span className="chat-edit-hint">Esc để hủy • Enter để lưu</span>
                          <div className="chat-edit-btns">
                            <button
                              type="button"
                              className="ws-btn ws-btn-ghost"
                              onClick={cancelEditing}
                              disabled={savingEdit}
                            >
                              Hủy
                            </button>
                            <button
                              type="button"
                              className="ws-btn ws-btn-primary"
                              onClick={() => void saveEdit(msg)}
                              disabled={savingEdit}
                            >
                              {savingEdit ? 'Đang lưu...' : 'Lưu'}
                            </button>
                          </div>
                        </div>
                      </div>
                    ) : (
                      <p className="chat-message-text">{msg.content}</p>
                    )}

                    {msg.optimisticStatus === 'failed' && (
                      <div className="chat-failed-bar" role="alert">
                        <span>{msg.errorMessage || 'Không thể gửi tin nhắn.'}</span>
                        <div className="chat-failed-actions">
                          <button
                            type="button"
                            className="chat-inline-action"
                            onClick={() => void onRetrySend(msg)}
                          >
                            Thử lại
                          </button>
                          <button
                            type="button"
                            className="chat-inline-action chat-inline-action-danger"
                            onClick={() => onDismissFailed(msg.nonce ?? msg.id)}
                          >
                            Xóa
                          </button>
                        </div>
                      </div>
                    )}
                  </div>

                  {(canEdit || canDelete) && !isEditingThis && (
                    <div className="chat-message-toolbar">
                      {canEdit && (
                        <button
                          type="button"
                          className="ws-channel-icon-btn"
                          onClick={() => startEditing(msg)}
                          title="Sửa tin nhắn"
                          aria-label="Sửa tin nhắn"
                        >
                          <EditIcon />
                        </button>
                      )}
                      {canDelete && (
                        <button
                          type="button"
                          className="ws-channel-icon-btn ws-channel-icon-btn-danger"
                          onClick={() => setDeleteTarget(msg)}
                          title="Xóa tin nhắn"
                          aria-label="Xóa tin nhắn"
                        >
                          <TrashIcon />
                        </button>
                      )}
                    </div>
                  )}
                </li>
              )
            })}
          </ol>
        )}
      </div>

      <ConfirmActionModal
        open={deleteTarget !== null}
        title="Xóa tin nhắn"
        description={
          deleteTarget
            ? `Bạn có chắc muốn xóa tin nhắn này của ${deleteTarget.author.displayName}?`
            : ''
        }
        confirmLabel="Xóa tin nhắn"
        isPending={isDeleting}
        onClose={() => setDeleteTarget(null)}
        onConfirm={async () => {
          if (deleteTarget) {
            await onDeleteMessage(deleteTarget.id)
          }
        }}
      />
    </>
  )
}
