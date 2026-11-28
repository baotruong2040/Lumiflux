import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { channelApi, guildApi } from '../../../core/api'
import type { Channel, GuildRole } from '../../../core/api'
import { gatewayClient } from '../../../core/gateway'
import { useAuthStore } from '../../auth/store/authStore'
import { useGatewayConnection } from '../../realtime/useGatewayConnection'
import { ChannelSidebar } from '../components/ChannelSidebar'
import { ChatArea } from '../components/ChatArea'
import { ServerBar } from '../components/ServerBar'
import {
  ChannelFormModal,
  ConfirmActionModal,
  CreateInviteModal,
  CreateServerModal,
  EditServerModal,
  JoinServerModal,
} from '../components/WorkspaceModals'
import { useWorkspaceStore } from '../store/workspaceStore'
import './WorkspaceLayout.css'

type ConfirmState =
  | { kind: 'delete-channel'; channel: Channel }
  | { kind: 'leave-guild'; guildId: string; guildName: string }
  | { kind: 'delete-guild'; guildId: string; guildName: string }
  | null

type ChannelModalState =
  | { mode: 'create'; channel: null }
  | { mode: 'edit'; channel: Channel }
  | null

export function WorkspacePage() {
  const { guildId: routeGuildId, channelId: routeChannelId } = useParams<{
    guildId?: string
    channelId?: string
  }>()
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  const user = useAuthStore((s) => s.user)
  const logout = useAuthStore((s) => s.logout)
  const gatewayStatus = useGatewayConnection(user)

  const showMembers = useWorkspaceStore((s) => s.showMembers)
  const toggleMembers = useWorkspaceStore((s) => s.toggleMembers)
  const setActiveGuild = useWorkspaceStore((s) => s.setActiveGuild)
  const setActiveChannel = useWorkspaceStore((s) => s.setActiveChannel)
  const resetWorkspace = useWorkspaceStore((s) => s.reset)

  const [createServerOpen, setCreateServerOpen] = useState(false)
  const [editServerOpen, setEditServerOpen] = useState(false)
  const [joinServerOpen, setJoinServerOpen] = useState(false)
  const [inviteOpen, setInviteOpen] = useState(false)
  const [channelModal, setChannelModal] = useState<ChannelModalState>(null)
  const [confirmState, setConfirmState] = useState<ConfirmState>(null)

  const guildsQuery = useQuery({
    queryKey: ['guilds'],
    queryFn: () => guildApi.listGuilds(),
    enabled: Boolean(user),
  })

  const guilds = useMemo(() => guildsQuery.data ?? [], [guildsQuery.data])

  const activeGuild = useMemo(() => {
    if (guilds.length === 0) return null
    if (routeGuildId) {
      const matched = guilds.find((g) => g.id === routeGuildId)
      if (matched) return matched
    }
    return guilds[0]
  }, [guilds, routeGuildId])

  const activeGuildId = activeGuild?.id ?? null

  const channelsQuery = useQuery({
    queryKey: ['channels', activeGuildId],
    queryFn: () => channelApi.listChannels(activeGuildId!),
    enabled: Boolean(activeGuildId),
  })

  const membersQuery = useQuery({
    queryKey: ['members', activeGuildId],
    queryFn: () => guildApi.listMembers(activeGuildId!),
    enabled: Boolean(activeGuildId),
  })

  const channels = useMemo(() => channelsQuery.data ?? [], [channelsQuery.data])
  const members = useMemo(() => membersQuery.data ?? [], [membersQuery.data])

  const activeChannel = useMemo(() => {
    if (channels.length === 0) return null
    if (routeChannelId) {
      const matched = channels.find((c) => c.id === routeChannelId)
      if (matched) return matched
    }
    return channels.find((c) => c.name === 'general') ?? channels[0]
  }, [channels, routeChannelId])

  const activeChannelId = activeChannel?.id ?? null

  const currentRole: GuildRole | null = useMemo(() => {
    if (!activeGuild || !user) return null
    const me = members.find((m) => m.userId === user.id)
    if (me) return me.role
    return activeGuild.ownerId === user.id ? 'owner' : 'member'
  }, [activeGuild, members, user])

  useEffect(() => {
    setActiveGuild(activeGuildId)
  }, [activeGuildId, setActiveGuild])

  useEffect(() => {
    setActiveChannel(activeChannelId)
  }, [activeChannelId, setActiveChannel])

  useEffect(() => {
    if (!guildsQuery.isSuccess) return

    if (guilds.length === 0) {
      if (routeGuildId || routeChannelId) {
        navigate('/app', { replace: true })
      }
      return
    }

    if (!activeGuildId) return

    if (channelsQuery.isSuccess) {
      if (activeChannelId) {
        if (routeGuildId !== activeGuildId || routeChannelId !== activeChannelId) {
          navigate(`/app/guilds/${activeGuildId}/channels/${activeChannelId}`, { replace: true })
        }
      } else if (routeGuildId !== activeGuildId || routeChannelId) {
        navigate(`/app/guilds/${activeGuildId}`, { replace: true })
      }
    } else if (routeGuildId !== activeGuildId) {
      navigate(`/app/guilds/${activeGuildId}`, { replace: true })
    }
  }, [
    guildsQuery.isSuccess,
    channelsQuery.isSuccess,
    guilds.length,
    activeGuildId,
    activeChannelId,
    routeGuildId,
    routeChannelId,
    navigate,
  ])

  const createServerMutation = useMutation({
    mutationFn: (payload: { name: string; iconUrl: string | null }) => guildApi.createGuild(payload),
    onSuccess: async (created) => {
      await queryClient.invalidateQueries({ queryKey: ['guilds'] })
      navigate(`/app/guilds/${created.id}`)
    },
  })

  const editServerMutation = useMutation({
    mutationFn: ({ guildId, payload }: { guildId: string; payload: { name: string; iconUrl: string | null } }) =>
      guildApi.updateGuild(guildId, payload),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['guilds'] })
    },
  })

  const joinServerMutation = useMutation({
    mutationFn: (code: string) => guildApi.acceptInvite(code),
    onSuccess: async (joined) => {
      await queryClient.invalidateQueries({ queryKey: ['guilds'] })
      await queryClient.invalidateQueries({ queryKey: ['members', joined.id] })
      navigate(`/app/guilds/${joined.id}`)
    },
  })

  const createInviteMutation = useMutation({
    mutationFn: ({
      guildId,
      payload,
    }: {
      guildId: string
      payload: { maxUses: number | null; expiresInHours: number | null }
    }) => guildApi.createInvite(guildId, payload),
  })

  const createChannelMutation = useMutation({
    mutationFn: ({ guildId, payload }: { guildId: string; payload: { name: string; topic: string | null } }) =>
      channelApi.createChannel(guildId, payload),
    onSuccess: async (created) => {
      await queryClient.invalidateQueries({ queryKey: ['channels', created.guildId] })
      navigate(`/app/guilds/${created.guildId}/channels/${created.id}`)
    },
  })

  const editChannelMutation = useMutation({
    mutationFn: ({ channelId, payload }: { channelId: string; payload: { name: string; topic: string | null } }) =>
      channelApi.updateChannel(channelId, payload),
    onSuccess: async (updated) => {
      await queryClient.invalidateQueries({ queryKey: ['channels', updated.guildId] })
    },
  })

  const deleteChannelMutation = useMutation({
    mutationFn: (channel: Channel) => channelApi.deleteChannel(channel.id),
    onSuccess: async (_, deleted) => {
      const remaining = channels.filter((c) => c.id !== deleted.id)
      await queryClient.invalidateQueries({ queryKey: ['channels', deleted.guildId] })
      if (activeChannelId === deleted.id) {
        const nextChannel = remaining.find((c) => c.name === 'general') ?? remaining[0]
        if (nextChannel) {
          navigate(`/app/guilds/${deleted.guildId}/channels/${nextChannel.id}`, { replace: true })
        } else {
          navigate(`/app/guilds/${deleted.guildId}`, { replace: true })
        }
      }
    },
  })

  const leaveGuildMutation = useMutation({
    mutationFn: (guildId: string) => guildApi.leaveGuild(guildId),
    onSuccess: async (_, leftGuildId) => {
      queryClient.removeQueries({ queryKey: ['channels', leftGuildId] })
      queryClient.removeQueries({ queryKey: ['members', leftGuildId] })
      await queryClient.invalidateQueries({ queryKey: ['guilds'] })
      const nextGuild = guilds.find((g) => g.id !== leftGuildId)
      navigate(nextGuild ? `/app/guilds/${nextGuild.id}` : '/app', { replace: true })
    },
  })

  const deleteGuildMutation = useMutation({
    mutationFn: (guildId: string) => guildApi.deleteGuild(guildId),
    onSuccess: async (_, deletedGuildId) => {
      queryClient.removeQueries({ queryKey: ['channels', deletedGuildId] })
      queryClient.removeQueries({ queryKey: ['members', deletedGuildId] })
      await queryClient.invalidateQueries({ queryKey: ['guilds'] })
      const nextGuild = guilds.find((g) => g.id !== deletedGuildId)
      navigate(nextGuild ? `/app/guilds/${nextGuild.id}` : '/app', { replace: true })
    },
  })

  async function handleLogout() {
    gatewayClient.disconnect()
    await logout()
    queryClient.clear()
    resetWorkspace()
    navigate('/login', { replace: true })
  }

  return (
    <div className="ws-shell-wrap">
      {gatewayStatus === 'reconnecting' && (
        <div className="ws-connection-banner" role="status" aria-live="polite">
          Mất kết nối realtime — đang tự động kết nối lại và đồng bộ tin nhắn...
        </div>
      )}
      <div className="ws-shell">
      <ServerBar
        guilds={guilds}
        activeGuildId={activeGuildId}
        onSelectGuild={(guildId) => navigate(`/app/guilds/${guildId}`)}
        onOpenCreateServer={() => setCreateServerOpen(true)}
        onOpenJoinServer={() => setJoinServerOpen(true)}
      />

      <ChannelSidebar
        user={user}
        activeGuild={activeGuild}
        role={currentRole}
        channels={channels}
        activeChannelId={activeChannelId}
        isLoadingChannels={channelsQuery.isLoading}
        onSelectChannel={(channelId) => {
          if (activeGuildId) {
            navigate(`/app/guilds/${activeGuildId}/channels/${channelId}`)
          }
        }}
        onOpenCreateChannel={() => setChannelModal({ mode: 'create', channel: null })}
        onOpenEditChannel={(channel) => setChannelModal({ mode: 'edit', channel })}
        onOpenDeleteChannel={(channel) => setConfirmState({ kind: 'delete-channel', channel })}
        onOpenInvite={() => setInviteOpen(true)}
        onOpenEditServer={() => setEditServerOpen(true)}
        onOpenLeaveServer={() => {
          if (activeGuild) {
            setConfirmState({ kind: 'leave-guild', guildId: activeGuild.id, guildName: activeGuild.name })
          }
        }}
        onOpenDeleteServer={() => {
          if (activeGuild) {
            setConfirmState({ kind: 'delete-guild', guildId: activeGuild.id, guildName: activeGuild.name })
          }
        }}
        onLogout={handleLogout}
      />

      <ChatArea
        currentUser={user}
        activeGuild={activeGuild}
        activeChannel={activeChannel}
        role={currentRole}
        members={members}
        isLoadingGuilds={guildsQuery.isLoading}
        isLoadingMembers={membersQuery.isLoading}
        showMembers={showMembers}
        onToggleMembers={toggleMembers}
        onOpenCreateServer={() => setCreateServerOpen(true)}
        onOpenJoinServer={() => setJoinServerOpen(true)}
        onOpenInvite={() => setInviteOpen(true)}
        onOpenCreateChannel={() => setChannelModal({ mode: 'create', channel: null })}
        onOpenEditChannel={(channel) => setChannelModal({ mode: 'edit', channel })}
      />

      <CreateServerModal
        open={createServerOpen}
        isPending={createServerMutation.isPending}
        onClose={() => setCreateServerOpen(false)}
        onSubmit={async (payload) => {
          await createServerMutation.mutateAsync(payload)
        }}
      />

      <EditServerModal
        open={editServerOpen}
        guild={activeGuild}
        isPending={editServerMutation.isPending}
        onClose={() => setEditServerOpen(false)}
        onSubmit={async (guildId, payload) => {
          await editServerMutation.mutateAsync({ guildId, payload })
        }}
      />

      <JoinServerModal
        open={joinServerOpen}
        isPending={joinServerMutation.isPending}
        onClose={() => setJoinServerOpen(false)}
        onSubmit={async (code) => {
          await joinServerMutation.mutateAsync(code)
        }}
      />

      <CreateInviteModal
        open={inviteOpen}
        guild={activeGuild}
        isPending={createInviteMutation.isPending}
        onClose={() => setInviteOpen(false)}
        onGenerate={(guildId, payload) => createInviteMutation.mutateAsync({ guildId, payload })}
      />

      <ChannelFormModal
        open={channelModal !== null}
        mode={channelModal?.mode ?? 'create'}
        channel={channelModal?.channel ?? null}
        isPending={createChannelMutation.isPending || editChannelMutation.isPending}
        onClose={() => setChannelModal(null)}
        onSubmit={async (payload) => {
          if (!channelModal) return
          if (channelModal.mode === 'create' && activeGuildId) {
            await createChannelMutation.mutateAsync({ guildId: activeGuildId, payload })
          } else if (channelModal.mode === 'edit' && channelModal.channel) {
            await editChannelMutation.mutateAsync({ channelId: channelModal.channel.id, payload })
          }
        }}
      />

      <ConfirmActionModal
        open={confirmState !== null}
        title={
          confirmState?.kind === 'delete-channel'
            ? `Xóa kênh #${confirmState.channel.name}`
            : confirmState?.kind === 'leave-guild'
              ? `Rời khỏi ${confirmState.guildName}`
              : confirmState?.kind === 'delete-guild'
                ? `Xóa server ${confirmState.guildName}`
                : ''
        }
        description={
          confirmState?.kind === 'delete-channel'
            ? `Bạn có chắc muốn xóa kênh #${confirmState.channel.name}? Hành động này sẽ xóa toàn bộ lịch sử kênh.`
            : confirmState?.kind === 'leave-guild'
              ? `Bạn sẽ rời khỏi server "${confirmState.guildName}" và cần mã mời mới nếu muốn tham gia lại.`
              : confirmState?.kind === 'delete-guild'
                ? `Bạn có chắc muốn xóa vĩnh viễn server "${confirmState.guildName}"? Mọi kênh và thành viên đều bị xóa.`
                : ''
        }
        confirmLabel={
          confirmState?.kind === 'leave-guild' ? 'Rời server' : 'Xác nhận xóa'
        }
        isPending={
          deleteChannelMutation.isPending ||
          leaveGuildMutation.isPending ||
          deleteGuildMutation.isPending
        }
        onClose={() => setConfirmState(null)}
        onConfirm={async () => {
          if (!confirmState) return
          if (confirmState.kind === 'delete-channel') {
            await deleteChannelMutation.mutateAsync(confirmState.channel)
          } else if (confirmState.kind === 'leave-guild') {
            await leaveGuildMutation.mutateAsync(confirmState.guildId)
          } else if (confirmState.kind === 'delete-guild') {
            await deleteGuildMutation.mutateAsync(confirmState.guildId)
          }
        }}
      />
      </div>
    </div>
  )
}
