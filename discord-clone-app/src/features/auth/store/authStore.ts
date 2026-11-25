import { create } from 'zustand'
import { ApiError, authApi, configureHttpClientAuth } from '../../../core/api'
import type { AuthResponse, LoginRequest, RegisterRequest, User } from '../../../core/api'
import { storage } from '../../../core/platform'

export const ACCESS_TOKEN_KEY = 'lumiflux.accessToken'
export const REFRESH_TOKEN_KEY = 'lumiflux.refreshToken'

export type AuthStatus = 'idle' | 'hydrating' | 'authenticated' | 'unauthenticated'

export interface AuthState {
  user: User | null
  accessToken: string | null
  refreshToken: string | null
  status: AuthStatus
  setSession: (auth: AuthResponse) => void
  setUser: (user: User) => void
  clearSession: () => void
  hydrate: () => Promise<void>
  login: (payload: LoginRequest) => Promise<AuthResponse>
  register: (payload: RegisterRequest) => Promise<AuthResponse>
  logout: () => Promise<void>
}

let hydrateInFlight: Promise<void> | null = null

export const useAuthStore = create<AuthState>((set, get) => ({
  user: null,
  accessToken: storage.getItem(ACCESS_TOKEN_KEY),
  refreshToken: storage.getItem(REFRESH_TOKEN_KEY),
  status: 'idle',

  setSession(auth) {
    storage.setItem(ACCESS_TOKEN_KEY, auth.accessToken)
    storage.setItem(REFRESH_TOKEN_KEY, auth.refreshToken)
    set({
      user: auth.user,
      accessToken: auth.accessToken,
      refreshToken: auth.refreshToken,
      status: 'authenticated',
    })
  },

  setUser(user) {
    set({ user })
  },

  clearSession() {
    storage.removeItem(ACCESS_TOKEN_KEY)
    storage.removeItem(REFRESH_TOKEN_KEY)
    set({
      user: null,
      accessToken: null,
      refreshToken: null,
      status: 'unauthenticated',
    })
  },

  async hydrate() {
    if (hydrateInFlight) {
      return hydrateInFlight
    }

    const storedAccess = storage.getItem(ACCESS_TOKEN_KEY)
    const storedRefresh = storage.getItem(REFRESH_TOKEN_KEY)

    if (!storedAccess && !storedRefresh) {
      set({
        user: null,
        accessToken: null,
        refreshToken: null,
        status: 'unauthenticated',
      })
      return
    }

    set({
      accessToken: storedAccess,
      refreshToken: storedRefresh,
      status: 'hydrating',
    })

    hydrateInFlight = (async () => {
      try {
        if (storedAccess) {
          const user = await authApi.getMe()
          const latest = get()
          set({
            user,
            accessToken: latest.accessToken ?? storage.getItem(ACCESS_TOKEN_KEY),
            refreshToken: latest.refreshToken ?? storage.getItem(REFRESH_TOKEN_KEY),
            status: 'authenticated',
          })
          return
        }

        if (storedRefresh) {
          const res = await authApi.refresh({ refreshToken: storedRefresh })
          get().setSession(res)
          return
        }

        get().clearSession()
      } catch {
        get().clearSession()
      } finally {
        hydrateInFlight = null
      }
    })()

    return hydrateInFlight
  },

  async login(payload) {
    const res = await authApi.login(payload)
    get().setSession(res)
    return res
  },

  async register(payload) {
    const res = await authApi.register(payload)
    get().setSession(res)
    return res
  },

  async logout() {
    const refreshToken = get().refreshToken ?? storage.getItem(REFRESH_TOKEN_KEY)
    try {
      if (refreshToken) {
        await authApi.logout({ refreshToken })
      }
    } catch (err) {
      if (!(err instanceof ApiError && err.status === 401)) {
        // Best-effort revoke; clear local session regardless.
      }
    } finally {
      get().clearSession()
    }
  },
}))

configureHttpClientAuth({
  getAccessToken: () => useAuthStore.getState().accessToken ?? storage.getItem(ACCESS_TOKEN_KEY),
  getRefreshToken: () => useAuthStore.getState().refreshToken ?? storage.getItem(REFRESH_TOKEN_KEY),
  onTokensRefreshed: (auth) => {
    useAuthStore.getState().setSession(auth)
  },
  onAuthFailure: () => {
    useAuthStore.getState().clearSession()
  },
})
