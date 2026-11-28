import { useRef, useState } from 'react'
import type { ChangeEvent, CompositionEvent, KeyboardEvent } from 'react'

export interface MessageComposerProps {
  channelName: string
  onSendMessage: (content: string) => Promise<void>
  onTyping?: () => void
}

const MAX_MESSAGE_LENGTH = 2000
const TYPING_THROTTLE_MS = 2500

export function MessageComposer({ channelName, onSendMessage, onTyping }: MessageComposerProps) {
  const [text, setText] = useState('')
  const [validationError, setValidationError] = useState<string | null>(null)
  const isComposingRef = useRef(false)
  const lastTypingSentRef = useRef(0)
  const textareaRef = useRef<HTMLTextAreaElement | null>(null)

  const trimmedLength = text.trim().length
  const overLimit = trimmedLength > MAX_MESSAGE_LENGTH

  function resizeTextarea(el: HTMLTextAreaElement) {
    el.style.height = 'auto'
    el.style.height = `${Math.min(el.scrollHeight, 180)}px`
  }

  function handleChange(e: ChangeEvent<HTMLTextAreaElement>) {
    const nextValue = e.target.value
    setText(nextValue)
    if (validationError) {
      setValidationError(null)
    }
    resizeTextarea(e.currentTarget)

    if (onTyping && nextValue.trim().length > 0) {
      const now = Date.now()
      if (now - lastTypingSentRef.current >= TYPING_THROTTLE_MS) {
        lastTypingSentRef.current = now
        onTyping()
      }
    }
  }

  function handleCompositionStart(_e: CompositionEvent<HTMLTextAreaElement>) {
    isComposingRef.current = true
  }

  function handleCompositionEnd(_e: CompositionEvent<HTMLTextAreaElement>) {
    isComposingRef.current = false
  }

  async function submitMessage() {
    const trimmed = text.trim()
    if (!trimmed) {
      setValidationError('Tin nhắn không được để trống.')
      return
    }
    if ([...trimmed].length > MAX_MESSAGE_LENGTH) {
      setValidationError('Tin nhắn tối đa 2.000 ký tự.')
      return
    }

    setValidationError(null)
    lastTypingSentRef.current = 0
    setText('')
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto'
    }

    try {
      await onSendMessage(trimmed)
    } catch {
      // Failed optimistic state is surfaced inline in MessageList with Retry/Dismiss controls.
    } finally {
      textareaRef.current?.focus()
    }
  }

  function handleKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Enter' && !e.shiftKey) {
      if (isComposingRef.current || e.nativeEvent.isComposing || e.keyCode === 229) {
        return
      }
      e.preventDefault()
      void submitMessage()
    }
  }

  return (
    <div className="chat-composer-wrap">
      {validationError && (
        <div className="chat-composer-error" role="alert">
          {validationError}
        </div>
      )}

      <div className="chat-composer-box">
        <textarea
          ref={textareaRef}
          rows={1}
          className="chat-composer-input"
          placeholder={`Nhắn tin vào #${channelName}`}
          aria-label={`Nhắn tin vào #${channelName}`}
          value={text}
          onChange={handleChange}
          onCompositionStart={handleCompositionStart}
          onCompositionEnd={handleCompositionEnd}
          onKeyDown={handleKeyDown}
        />

        <div className="chat-composer-right">
          {text.length >= 1600 && (
            <span className={`chat-char-count ${overLimit ? 'chat-char-count-over' : ''}`}>
              {[...text.trim()].length}/{MAX_MESSAGE_LENGTH}
            </span>
          )}
          <button
            type="button"
            className="ws-btn ws-btn-primary chat-send-btn"
            onClick={() => void submitMessage()}
            disabled={!trimmedLength || overLimit}
          >
            Gửi
          </button>
        </div>
      </div>

      <p className="chat-composer-hint">
        Nhấn <kbd>Enter</kbd> để gửi, <kbd>Shift + Enter</kbd> để xuống dòng (hỗ trợ gõ tiếng Việt Telex/VNI).
      </p>
    </div>
  )
}
