import type { Channel, Guild, GuildMember, GuildRole, User } from '../../../core/api'
import { MessageComposer } from '../../chat/components/MessageComposer'
import { MessageList } from '../../chat/components/MessageList'
import '../../chat/components/ChatStyles.css'
import { useChannelMessages } from '../../chat/hooks/useChannelMessages'
import { getInitials } from '../utils/workspaceHelpers'
import { HashIcon, PlusIcon, UserPlusIcon, UsersIcon } from './Icons'

export interface ChatAreaProps {
  currentUser: User | null
  activeGuild: Guild | null
  activeChannel: Channel | null
  role: GuildRole | null
  members: GuildMember[]
  isLoadingGuilds: boolean
  isLoadingMembers: boolean
  showMembers: boolean
  onToggleMembers: () => void
  onOpenCreateServer: () => void
  onOpenJoinServer: () => void
  onOpenInvite: () => void
  onOpenCreateChannel: () => void
  onOpenEditChannel: (channel: Channel) => void
}

const ROLE_LABELS: Record<GuildRole, string> = {
  owner: 'Owner',
  admin: 'Admin',
  member: 'Member',
}

export function ChatArea({
  currentUser,
  activeGuild,
  activeChannel,
  role,
  members,
  isLoadingGuilds,
  isLoadingMembers,
  showMembers,
  onToggleMembers,
  onOpenCreateServer,
  onOpenJoinServer,
  onOpenInvite,
  onOpenCreateChannel,
  onOpenEditChannel,
}: ChatAreaProps) {
  const canManageChannels = role === 'owner' || role === 'admin'
  const chat = useChannelMessages(activeChannel?.id ?? null, currentUser)

  if (!activeGuild) {
    return (
      <main className="ws-chat-area ws-chat-empty-state">
        <div className="ws-welcome-card">
          <span className="ws-welcome-badge">Lumiflux Dark Workspace</span>
          <h1 className="ws-welcome-title">
            {isLoadingGuilds ? 'Đang tải không gian làm việc...' : 'Chào mừng đến với Lumiflux'}
          </h1>
          <p className="ws-welcome-desc">
            {isLoadingGuilds
              ? 'Vui lòng chờ trong giây lát để đồng bộ danh sách server của bạn.'
              : 'Bạn chưa tham gia server nào. Hãy tạo server mới hoặc nhập mã mời để bắt đầu trò chuyện cùng nhóm.'}
          </p>
          {!isLoadingGuilds && (
            <div className="ws-welcome-actions">
              <button type="button" className="ws-btn ws-btn-primary" onClick={onOpenCreateServer}>
                Tạo server mới
              </button>
              <button type="button" className="ws-btn ws-btn-success" onClick={onOpenJoinServer}>
                Tham gia bằng mã mời
              </button>
            </div>
          )}
        </div>
      </main>
    )
  }

  return (
    <main className="ws-chat-area">
      <header className="ws-chat-header">
        <div className="ws-chat-header-left">
          <HashIcon className="ws-chat-header-hash" />
          <h1 className="ws-chat-channel-title">
            {activeChannel ? activeChannel.name : activeGuild.name}
          </h1>
          {activeChannel?.topic && (
            <>
              <span className="ws-chat-topic-sep" aria-hidden="true" />
              <p className="ws-chat-topic" title={activeChannel.topic}>
                {activeChannel.topic}
              </p>
            </>
          )}
        </div>

        <div className="ws-chat-header-actions">
          <button
            type="button"
            className="ws-header-btn"
            onClick={onOpenInvite}
            title="Tạo mã mời"
          >
            <UserPlusIcon />
            <span>Mời bạn bè</span>
          </button>

          <button
            type="button"
            className={`ws-header-btn ${showMembers ? 'ws-header-btn-active' : ''}`}
            onClick={onToggleMembers}
            aria-pressed={showMembers}
            title="Ẩn/hiện danh sách thành viên"
          >
            <UsersIcon />
            <span>Thành viên ({members.length})</span>
          </button>
        </div>
      </header>

      <div className="ws-chat-body-split">
        <section className="ws-chat-content" aria-label="Khung nội dung kênh">
          {!activeChannel ? (
            <div className="ws-channel-placeholder">
              <h2>Server chưa có kênh nào</h2>
              <p>
                {canManageChannels
                  ? 'Hãy tạo kênh văn bản đầu tiên để các thành viên có thể giao tiếp.'
                  : 'Vui lòng chờ Owner hoặc Admin tạo kênh trong server này.'}
              </p>
              {canManageChannels && (
                <button
                  type="button"
                  className="ws-btn ws-btn-primary ws-btn-icon-text"
                  onClick={onOpenCreateChannel}
                >
                  <PlusIcon width={16} height={16} />
                  <span>Tạo kênh văn bản</span>
                </button>
              )}
            </div>
          ) : (
            <>
              <MessageList
                key={activeChannel.id}
                guildName={activeGuild.name}
                channel={activeChannel}
                currentUser={currentUser}
                role={role}
                messages={chat.messages}
                isLoading={chat.isLoading}
                isError={chat.isError}
                error={chat.error}
                hasNextPage={chat.hasNextPage}
                isFetchingNextPage={chat.isFetchingNextPage}
                onFetchNextPage={chat.fetchNextPage}
                onRetryLoad={chat.refetch}
                onRetrySend={chat.retryMessage}
                onDismissFailed={chat.dismissFailedMessage}
                onEditMessage={chat.editMessage}
                onDeleteMessage={chat.deleteMessage}
                isDeleting={chat.isDeleting}
                onOpenEditChannel={onOpenEditChannel}
              />

              <MessageComposer
                key={`composer-${activeChannel.id}`}
                channelName={activeChannel.name}
                onSendMessage={chat.sendMessage}
              />
            </>
          )}
        </section>

        {showMembers && (
          <aside className="ws-member-panel" aria-label="Danh sách thành viên server">
            <h2 className="ws-member-heading">Thành viên — {members.length}</h2>
            {isLoadingMembers ? (
              <p className="ws-member-loading">Đang tải thành viên...</p>
            ) : (
              <ul className="ws-member-list">
                {members.map((member) => (
                  <li key={member.userId} className="ws-member-item">
                    <div className="ws-member-avatar">
                      {member.user.avatarUrl ? (
                        <img
                          src={member.user.avatarUrl}
                          alt={member.user.displayName}
                          className="ws-user-avatar-img"
                        />
                      ) : (
                        <span>{getInitials(member.user.displayName || member.user.username)}</span>
                      )}
                      <span
                        className={`ws-presence-dot ${
                          member.online ? 'ws-presence-online' : 'ws-presence-offline'
                        }`}
                        title={member.online ? 'Đang trực tuyến' : 'Ngoại tuyến'}
                      />
                    </div>
                    <div className="ws-member-info">
                      <span className="ws-member-name">
                        {member.nickname || member.user.displayName}
                      </span>
                      <span className="ws-member-username">@{member.user.username}</span>
                    </div>
                    <span className={`ws-role-badge ws-role-${member.role}`}>
                      {ROLE_LABELS[member.role]}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </aside>
        )}
      </div>
    </main>
  )
}
