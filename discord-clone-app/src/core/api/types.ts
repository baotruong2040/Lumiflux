export interface User {
  id: string
  username: string
  email: string
  displayName: string
  avatarUrl: string | null
  createdAt: string
}

export interface AuthResponse {
  accessToken: string
  refreshToken: string
  user: User
}

export interface RegisterRequest {
  username: string
  email: string
  displayName: string
  password: string
}

export interface LoginRequest {
  login: string
  password: string
}

export interface RefreshRequest {
  refreshToken: string
}

export interface LogoutRequest {
  refreshToken: string
}

export interface UpdateMeRequest {
  displayName?: string
  avatarUrl?: string | null
}

export interface Guild {
  id: string
  name: string
  iconUrl: string | null
  ownerId: string
  createdAt: string
}

export type GuildRole = 'owner' | 'admin' | 'member'

export interface GuildMember {
  guildId: string
  userId: string
  nickname: string | null
  role: GuildRole
  online: boolean
  joinedAt: string
  user: User
}

export interface Invite {
  code: string
  guildId: string
  createdBy: string
  maxUses: number | null
  uses: number
  expiresAt: string | null
  createdAt: string
}

export interface ApiErrorEnvelope {
  error: {
    code: string
    message: string
  }
}

export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly retryAfter?: number

  constructor(status: number, code: string, message: string, retryAfter?: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.retryAfter = retryAfter
  }
}
