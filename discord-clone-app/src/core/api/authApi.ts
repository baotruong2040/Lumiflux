import { httpClient } from './httpClient'
import type {
  AuthResponse,
  LoginRequest,
  LogoutRequest,
  RefreshRequest,
  RegisterRequest,
  UpdateMeRequest,
  User,
} from './types'

export const authApi = {
  register(payload: RegisterRequest): Promise<AuthResponse> {
    return httpClient.post<AuthResponse>('/api/v1/auth/register', payload, {
      skipAuth: true,
      skipRefresh: true,
    })
  },

  login(payload: LoginRequest): Promise<AuthResponse> {
    return httpClient.post<AuthResponse>('/api/v1/auth/login', payload, {
      skipAuth: true,
      skipRefresh: true,
    })
  },

  refresh(payload: RefreshRequest): Promise<AuthResponse> {
    return httpClient.post<AuthResponse>('/api/v1/auth/refresh', payload, {
      skipAuth: true,
      skipRefresh: true,
    })
  },

  logout(payload: LogoutRequest): Promise<void> {
    return httpClient.post<void>('/api/v1/auth/logout', payload, {
      skipRefresh: true,
    })
  },

  getMe(): Promise<User> {
    return httpClient.get<User>('/api/v1/users/@me')
  },

  updateMe(payload: UpdateMeRequest): Promise<User> {
    return httpClient.patch<User>('/api/v1/users/@me', payload)
  },
}
