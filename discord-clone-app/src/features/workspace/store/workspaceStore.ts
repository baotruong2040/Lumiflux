import { create } from 'zustand'

export interface WorkspaceState {
  activeGuildId: string | null
  activeChannelId: string | null
  showMembers: boolean
  setActiveGuild: (guildId: string | null) => void
  setActiveChannel: (channelId: string | null) => void
  toggleMembers: () => void
  reset: () => void
}

export const useWorkspaceStore = create<WorkspaceState>((set) => ({
  activeGuildId: null,
  activeChannelId: null,
  showMembers: true,

  setActiveGuild(guildId) {
    set({ activeGuildId: guildId })
  },

  setActiveChannel(channelId) {
    set({ activeChannelId: channelId })
  },

  toggleMembers() {
    set((s) => ({ showMembers: !s.showMembers }))
  },

  reset() {
    set({
      activeGuildId: null,
      activeChannelId: null,
      showMembers: true,
    })
  },
}))
