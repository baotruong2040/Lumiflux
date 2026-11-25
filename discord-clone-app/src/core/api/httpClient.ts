import { ApiError } from './types'
import type { ApiErrorEnvelope, AuthResponse } from './types'

export interface HttpClientAuthHooks {
  getAccessToken: () => string | null
  getRefreshToken: () => string | null
  onTokensRefreshed: (res: AuthResponse) => void
  onAuthFailure: () => void
}

export interface RequestOptions {
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  body?: unknown
  headers?: Record<string, string>
  signal?: AbortSignal
  skipAuth?: boolean
  skipRefresh?: boolean
}

export interface CreateHttpClientOptions {
  baseUrl: string
  fetchImpl?: typeof fetch
  authHooks?: HttpClientAuthHooks
}

export interface HttpClient {
  setAuthHooks(hooks: HttpClientAuthHooks): void
  request<T>(path: string, options?: RequestOptions): Promise<T>
  get<T>(path: string, options?: Omit<RequestOptions, 'method' | 'body'>): Promise<T>
  post<T>(path: string, body?: unknown, options?: Omit<RequestOptions, 'method' | 'body'>): Promise<T>
  patch<T>(path: string, body?: unknown, options?: Omit<RequestOptions, 'method' | 'body'>): Promise<T>
  delete<T>(path: string, options?: Omit<RequestOptions, 'method' | 'body'>): Promise<T>
}

const DEFAULT_BASE_URL =
  (import.meta.env?.VITE_API_URL as string | undefined) || 'http://localhost:8080'

function trimTrailingSlash(url: string): string {
  return url.endsWith('/') ? url.slice(0, -1) : url
}

async function parseErrorResponse(res: Response): Promise<ApiError> {
  const retryHeader = res.headers.get('Retry-After')
  const retryAfter = retryHeader ? Number.parseInt(retryHeader, 10) : undefined
  const validRetryAfter = Number.isFinite(retryAfter) ? retryAfter : undefined

  try {
    const data = (await res.json()) as Partial<ApiErrorEnvelope>
    if (data?.error?.code && data?.error?.message) {
      return new ApiError(res.status, data.error.code, data.error.message, validRetryAfter)
    }
  } catch {
    // Response body is not JSON; fall through to default error.
  }

  return new ApiError(
    res.status,
    res.status === 401 ? 'UNAUTHORIZED' : 'HTTP_ERROR',
    res.statusText || `Request failed with status ${res.status}`,
    validRetryAfter,
  )
}

export function createHttpClient(options: CreateHttpClientOptions): HttpClient {
  const baseUrl = trimTrailingSlash(options.baseUrl)
  const fetchFn: typeof fetch = options.fetchImpl ?? ((input, init) => globalThis.fetch(input, init))
  let hooks: HttpClientAuthHooks | undefined = options.authHooks
  let refreshPromise: Promise<AuthResponse> | null = null

  async function refreshSession(): Promise<AuthResponse> {
    if (refreshPromise) {
      return refreshPromise
    }

    const refreshToken = hooks?.getRefreshToken()
    if (!refreshToken) {
      throw new ApiError(401, 'INVALID_REFRESH_TOKEN', 'Missing refresh token')
    }

    refreshPromise = (async () => {
      const res = await fetchFn(`${baseUrl}/api/v1/auth/refresh`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refreshToken }),
      })

      if (!res.ok) {
        throw await parseErrorResponse(res)
      }

      const authRes = (await res.json()) as AuthResponse
      hooks?.onTokensRefreshed(authRes)
      return authRes
    })()

    try {
      return await refreshPromise
    } finally {
      refreshPromise = null
    }
  }

  async function executeRequest<T>(
    path: string,
    opts: RequestOptions,
    isRetry: boolean,
  ): Promise<T> {
    const method = opts.method ?? 'GET'
    const headers: Record<string, string> = {
      ...opts.headers,
    }

    if (opts.body !== undefined) {
      headers['Content-Type'] = 'application/json'
    }

    if (!opts.skipAuth) {
      const token = hooks?.getAccessToken()
      if (token) {
        headers['Authorization'] = `Bearer ${token}`
      }
    }

    const normalizedPath = path.startsWith('/') ? path : `/${path}`
    const res = await fetchFn(`${baseUrl}${normalizedPath}`, {
      method,
      headers,
      body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
      signal: opts.signal,
    })

    if (res.status === 401 && !opts.skipAuth && !opts.skipRefresh && !isRetry) {
      const refreshToken = hooks?.getRefreshToken()
      if (!refreshToken) {
        hooks?.onAuthFailure()
        throw await parseErrorResponse(res)
      }

      try {
        await refreshSession()
      } catch (refreshErr) {
        hooks?.onAuthFailure()
        throw refreshErr
      }

      return executeRequest<T>(path, opts, true)
    }

    if (!res.ok) {
      throw await parseErrorResponse(res)
    }

    if (res.status === 204) {
      return undefined as T
    }

    const contentType = res.headers.get('Content-Type') ?? ''
    if (!contentType.includes('application/json')) {
      return undefined as T
    }

    return (await res.json()) as T
  }

  return {
    setAuthHooks(nextHooks) {
      hooks = nextHooks
    },
    request(path, opts = {}) {
      return executeRequest(path, opts, false)
    },
    get(path, opts) {
      return executeRequest(path, { ...opts, method: 'GET' }, false)
    },
    post(path, body, opts) {
      return executeRequest(path, { ...opts, method: 'POST', body }, false)
    },
    patch(path, body, opts) {
      return executeRequest(path, { ...opts, method: 'PATCH', body }, false)
    },
    delete(path, opts) {
      return executeRequest(path, { ...opts, method: 'DELETE' }, false)
    },
  }
}

export const httpClient = createHttpClient({
  baseUrl: DEFAULT_BASE_URL,
})

export function configureHttpClientAuth(hooks: HttpClientAuthHooks): void {
  httpClient.setAuthHooks(hooks)
}
