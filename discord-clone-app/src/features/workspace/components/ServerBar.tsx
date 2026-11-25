import type { Guild } from '../../../core/api'
import { getInitials } from '../utils/workspaceHelpers'
import { CompassIcon, PlusIcon } from './Icons'

export interface ServerBarProps {
  guilds: Guild[]
  activeGuildId: string | null
  onSelectGuild: (guildId: string) => void
  onOpenCreateServer: () => void
  onOpenJoinServer: () => void
}

export function ServerBar({
  guilds,
  activeGuildId,
  onSelectGuild,
  onOpenCreateServer,
  onOpenJoinServer,
}: ServerBarProps) {
  return (
    <nav className="ws-server-bar" aria-label="Danh sách server">
      <div className="ws-server-brand" title="Lumiflux">
        <span>LF</span>
      </div>

      <div className="ws-server-divider" role="separator" />

      <div className="ws-server-list">
        {guilds.map((guild) => {
          const isActive = guild.id === activeGuildId
          return (
            <div key={guild.id} className="ws-server-item">
              <span className={`ws-server-pill ${isActive ? 'ws-server-pill-active' : ''}`} />
              <button
                type="button"
                className={`ws-server-btn ${isActive ? 'ws-server-btn-active' : ''}`}
                onClick={() => onSelectGuild(guild.id)}
                title={guild.name}
                aria-label={`Chọn server ${guild.name}`}
                aria-pressed={isActive}
              >
                {guild.iconUrl ? (
                  <img src={guild.iconUrl} alt={guild.name} className="ws-server-avatar" />
                ) : (
                  <span>{getInitials(guild.name)}</span>
                )}
              </button>
            </div>
          )
        })}
      </div>

      <div className="ws-server-divider" role="separator" />

      <button
        type="button"
        className="ws-server-action ws-server-action-add"
        onClick={onOpenCreateServer}
        title="Tạo server mới"
        aria-label="Tạo server mới"
      >
        <PlusIcon width={22} height={22} />
      </button>

      <button
        type="button"
        className="ws-server-action ws-server-action-join"
        onClick={onOpenJoinServer}
        title="Tham gia server bằng mã mời"
        aria-label="Tham gia server bằng mã mời"
      >
        <CompassIcon width={20} height={20} />
      </button>
    </nav>
  )
}
