import { useEffect, useRef, useState } from 'react'
import type { Channel, Guild, GuildRole, User } from '../../../core/api'
import { isTauriRuntime } from '../../../core/platform'
import { getInitials } from '../utils/workspaceHelpers'
import {
  ChevronDownIcon,
  EditIcon,
  HashIcon,
  LogOutIcon,
  PlusIcon,
  SettingsIcon,
  TrashIcon,
  UserPlusIcon,
} from './Icons'

export interface ChannelSidebarProps {
  user: User | null
  activeGuild: Guild | null
  role: GuildRole | null
  channels: Channel[]
  activeChannelId: string | null
  isLoadingChannels: boolean
  onSelectChannel: (channelId: string) => void
  onOpenCreateChannel: () => void
  onOpenEditChannel: (channel: Channel) => void
  onOpenDeleteChannel: (channel: Channel) => void
  onOpenInvite: () => void
  onOpenEditServer: () => void
  onOpenLeaveServer: () => void
  onOpenDeleteServer: () => void
  onLogout: () => void
}

export function ChannelSidebar({
  user,
  activeGuild,
  role,
  channels,
  activeChannelId,
  isLoadingChannels,
  onSelectChannel,
  onOpenCreateChannel,
  onOpenEditChannel,
  onOpenDeleteChannel,
  onOpenInvite,
  onOpenEditServer,
  onOpenLeaveServer,
  onOpenDeleteServer,
  onLogout,
}: ChannelSidebarProps) {
  const [menuGuildId, setMenuGuildId] = useState<string | null>(null)
  const menuRef = useRef<HTMLDivElement | null>(null)

  const menuOpen = Boolean(activeGuild && menuGuildId === activeGuild.id)
  const canManageChannels = role === 'owner' || role === 'admin'
  const isOwner = role === 'owner'
  const platformLabel = isTauriRuntime() ? 'Desktop' : 'Web'

  useEffect(() => {
    if (!menuOpen) return

    function handleClickOutside(e: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setMenuGuildId(null)
      }
    }

    window.addEventListener('mousedown', handleClickOutside)
    return () => window.removeEventListener('mousedown', handleClickOutside)
  }, [menuOpen])

  return (
    <aside className="ws-channel-sidebar" aria-label="Thanh bên kênh">
      <div className="ws-guild-header-wrap" ref={menuRef}>
        {activeGuild ? (
          <button
            type="button"
            className="ws-guild-header"
            onClick={() => setMenuGuildId((prev) => (prev === activeGuild.id ? null : activeGuild.id))}
            aria-expanded={menuOpen}
            aria-haspopup="menu"
          >
            <span className="ws-guild-header-name">{activeGuild.name}</span>
            <ChevronDownIcon className={`ws-guild-chevron ${menuOpen ? 'ws-guild-chevron-open' : ''}`} />
          </button>
        ) : (
          <div className="ws-guild-header ws-guild-header-empty">
            <span className="ws-guild-header-name">Lumiflux</span>
          </div>
        )}

        {menuOpen && activeGuild && (
          <div className="ws-guild-menu" role="menu">
            <button
              type="button"
              className="ws-guild-menu-item ws-guild-menu-accent"
              role="menuitem"
              onClick={() => {
                setMenuGuildId(null)
                onOpenInvite()
              }}
            >
              <span>Mời mọi người</span>
              <UserPlusIcon />
            </button>

            {canManageChannels && (
              <button
                type="button"
                className="ws-guild-menu-item"
                role="menuitem"
                onClick={() => {
                  setMenuGuildId(null)
                  onOpenCreateChannel()
                }}
              >
                <span>Tạo kênh mới</span>
                <PlusIcon width={16} height={16} />
              </button>
            )}

            {isOwner && (
              <button
                type="button"
                className="ws-guild-menu-item"
                role="menuitem"
                onClick={() => {
                  setMenuGuildId(null)
                  onOpenEditServer()
                }}
              >
                <span>Chỉnh sửa server</span>
                <SettingsIcon />
              </button>
            )}

            <div className="ws-guild-menu-sep" role="separator" />

            {isOwner ? (
              <button
                type="button"
                className="ws-guild-menu-item ws-guild-menu-danger"
                role="menuitem"
                onClick={() => {
                  setMenuGuildId(null)
                  onOpenDeleteServer()
                }}
              >
                <span>Xóa server</span>
                <TrashIcon />
              </button>
            ) : (
              <button
                type="button"
                className="ws-guild-menu-item ws-guild-menu-danger"
                role="menuitem"
                onClick={() => {
                  setMenuGuildId(null)
                  onOpenLeaveServer()
                }}
              >
                <span>Rời server</span>
                <LogOutIcon />
              </button>
            )}
          </div>
        )}
      </div>

      <div className="ws-channel-scroll">
        {!activeGuild ? (
          <div className="ws-channel-empty-hint">
            <p>Hãy tạo hoặc tham gia một server ở thanh bên trái để bắt đầu.</p>
          </div>
        ) : (
          <section className="ws-channel-group" aria-label="Danh sách kênh văn bản">
            <div className="ws-channel-group-header">
              <span>Kênh văn bản</span>
              {canManageChannels && (
                <button
                  type="button"
                  className="ws-channel-add-btn"
                  onClick={onOpenCreateChannel}
                  title="Tạo kênh mới"
                  aria-label="Tạo kênh mới"
                >
                  <PlusIcon width={15} height={15} />
                </button>
              )}
            </div>

            {isLoadingChannels ? (
              <div className="ws-channel-loading">Đang tải danh sách kênh...</div>
            ) : channels.length === 0 ? (
              <div className="ws-channel-loading">Chưa có kênh văn bản nào.</div>
            ) : (
              <ul className="ws-channel-list">
                {channels.map((channel) => {
                  const isActive = channel.id === activeChannelId
                  return (
                    <li
                      key={channel.id}
                      className={`ws-channel-row ${isActive ? 'ws-channel-row-active' : ''}`}
                    >
                      <button
                        type="button"
                        className="ws-channel-btn"
                        onClick={() => onSelectChannel(channel.id)}
                        aria-current={isActive ? 'page' : undefined}
                      >
                        <HashIcon className="ws-channel-hash" />
                        <span className="ws-channel-name">{channel.name}</span>
                      </button>

                      {canManageChannels && (
                        <div className="ws-channel-actions">
                          <button
                            type="button"
                            className="ws-channel-icon-btn"
                            onClick={() => onOpenEditChannel(channel)}
                            title={`Sửa kênh #${channel.name}`}
                            aria-label={`Sửa kênh #${channel.name}`}
                          >
                            <EditIcon />
                          </button>
                          <button
                            type="button"
                            className="ws-channel-icon-btn ws-channel-icon-btn-danger"
                            onClick={() => onOpenDeleteChannel(channel)}
                            title={`Xóa kênh #${channel.name}`}
                            aria-label={`Xóa kênh #${channel.name}`}
                          >
                            <TrashIcon />
                          </button>
                        </div>
                      )}
                    </li>
                  )
                })}
              </ul>
            )}
          </section>
        )}
      </div>

      <footer className="ws-user-panel">
        <div className="ws-user-meta">
          <div className="ws-user-avatar" aria-hidden="true">
            {user?.avatarUrl ? (
              <img src={user.avatarUrl} alt={user.displayName} className="ws-user-avatar-img" />
            ) : (
              <span>{getInitials(user?.displayName ?? user?.username ?? 'U')}</span>
            )}
            <span className="ws-presence-dot ws-presence-online" />
          </div>
          <div className="ws-user-names">
            <span className="ws-user-display">{user?.displayName ?? 'Người dùng'}</span>
            <span className="ws-user-sub">
              @{user?.username ?? 'unknown'} • {platformLabel}
            </span>
          </div>
        </div>

        <button
          type="button"
          className="ws-logout-btn"
          onClick={onLogout}
          title="Đăng xuất"
          aria-label="Đăng xuất"
        >
          <LogOutIcon />
        </button>
      </footer>
    </aside>
  )
}
