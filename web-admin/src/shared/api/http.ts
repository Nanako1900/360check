import axios, {
  type AxiosError,
  type AxiosInstance,
  type AxiosRequestConfig,
  type AxiosResponse,
  type InternalAxiosRequestConfig,
} from 'axios'
import { env } from '@/env'
import { useAuthStore } from '@/shared/auth/authStore'
import { sessionBus } from '@/shared/auth/sessionBus'
import { ApiError } from './apiError'
import type { ApiErrorCode } from './apiError'
import type { AuthTokens, Envelope, Paginated } from './types'

/** 标记请求跳过 Bearer 注入 / 跳过 401 刷新（用于 refresh 自身，避免递归）。 */
export const SKIP_AUTH_HEADER = 'X-Skip-Auth'
export const SKIP_REFRESH_HEADER = 'X-Skip-Refresh'

type RetriableConfig = InternalAxiosRequestConfig & { _retried?: boolean }

export const apiClient: AxiosInstance = axios.create({
  baseURL: env.VITE_API_BASE_URL,
  timeout: 30_000,
  headers: { 'Content-Type': 'application/json' },
})

// —— 请求拦截：附加 Bearer access token（§4.2 职责 1）——
apiClient.interceptors.request.use((config) => {
  if (config.headers[SKIP_AUTH_HEADER]) {
    delete config.headers[SKIP_AUTH_HEADER]
    return config
  }
  const token = useAuthStore.getState().accessToken
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

// —— 401 单飞刷新（single-flight）——
let refreshPromise: Promise<string> | null = null

async function performRefresh(): Promise<string> {
  const refreshToken = useAuthStore.getState().refreshToken
  if (!refreshToken) {
    throw new ApiError('UNAUTHENTICATED', '会话已失效，请重新登录', [], 401)
  }
  // 裸 axios：refresh 请求不再走会触发刷新的拦截器（§P0 关键坑）
  const resp = await axios.post<Envelope<AuthTokens>>(
    `${env.VITE_API_BASE_URL}/auth/refresh`,
    { refresh_token: refreshToken },
    { headers: { 'Content-Type': 'application/json' } },
  )
  const body = resp.data
  if (!body.success || !body.data) {
    throw new ApiError(
      body.error?.code ?? 'UNAUTHENTICATED',
      body.error?.message ?? '刷新令牌失败',
      body.error?.details ?? [],
      resp.status,
    )
  }
  // 轮换后的 access + refresh + user 一并落库
  useAuthStore.getState().setTokens(body.data)
  return body.data.access_token
}

/** 并发 401 只发一次 refresh，其余请求挂起等待新 token 后重放。 */
function refreshSingleFlight(): Promise<string> {
  if (!refreshPromise) {
    refreshPromise = performRefresh().finally(() => {
      refreshPromise = null
    })
  }
  return refreshPromise
}

// —— 响应拦截：401 按 error.code 分支（§4.6）——
apiClient.interceptors.response.use(
  (resp) => resp,
  async (error: AxiosError<Envelope<unknown>>) => {
    const original = error.config as RetriableConfig | undefined
    const status = error.response?.status
    const code = error.response?.data?.error?.code

    // TOKEN_EXPIRED → 单飞 refresh 后透明重放
    if (
      status === 401 &&
      code === 'TOKEN_EXPIRED' &&
      original &&
      !original._retried &&
      !original.headers?.[SKIP_REFRESH_HEADER]
    ) {
      original._retried = true
      let newToken: string
      try {
        newToken = await refreshSingleFlight()
      } catch (refreshError) {
        useAuthStore.getState().reset()
        sessionBus.emitForceLogout('REFRESH_FAILED')
        return Promise.reject(
          refreshError instanceof ApiError ? refreshError : normalizeError(error),
        )
      }
      original.headers.Authorization = `Bearer ${newToken}`
      // 重放：若仍 401（如时钟漂移/令牌被撤销），递归进入下方 _retried 登出分支（不再重复刷新）
      return apiClient(original)
    }

    // UNAUTHENTICATED → 直接登出；刷新后重放仍 401（original._retried）= 不可恢复 → 同样登出，
    // 避免停留在半登录态持续失败（_retried 守卫已防止无限刷新循环）。
    if (status === 401 && (code === 'UNAUTHENTICATED' || original?._retried)) {
      useAuthStore.getState().reset()
      sessionBus.emitForceLogout('UNAUTHENTICATED')
    }

    return Promise.reject(normalizeError(error))
  },
)

/** 把 AxiosError 规整为 ApiError（带信封 error.code，或网络/超时兜底）。 */
function normalizeError(error: AxiosError<Envelope<unknown>>): ApiError {
  const status = error.response?.status
  const errObj = error.response?.data?.error
  if (errObj?.code) {
    return new ApiError(
      errObj.code as ApiErrorCode,
      errObj.message ?? '请求失败',
      errObj.details ?? [],
      status,
    )
  }
  if (error.code === 'ECONNABORTED') {
    return new ApiError('INTERNAL', '请求超时，请稍后重试', [], status)
  }
  return new ApiError('INTERNAL', error.message || '网络异常，请检查网络连接', [], status)
}

/** 成功信封拆封：业务层拿到的是 T，不是信封（§4.2 拦截器职责 2）。 */
function unwrap<T>(resp: AxiosResponse<Envelope<T>>): T {
  const body = resp.data
  if (!body.success) {
    throw new ApiError(
      body.error?.code ?? 'INTERNAL',
      body.error?.message ?? '请求失败',
      body.error?.details ?? [],
      resp.status,
    )
  }
  return body.data as T
}

export async function request<T>(config: AxiosRequestConfig): Promise<T> {
  const resp = await apiClient.request<Envelope<T>>(config)
  return unwrap(resp)
}

/** 列表门面：拆封 data + 规整 meta 为 { items, total, page, pageSize }。 */
export async function requestList<T>(config: AxiosRequestConfig): Promise<Paginated<T>> {
  const resp = await apiClient.request<Envelope<T[]>>(config)
  const body = resp.data
  if (!body.success) {
    throw new ApiError(
      body.error?.code ?? 'INTERNAL',
      body.error?.message ?? '请求失败',
      body.error?.details ?? [],
      resp.status,
    )
  }
  const items = body.data ?? []
  return {
    items,
    total: body.meta?.total ?? items.length,
    page: body.meta?.page ?? 1,
    pageSize: body.meta?.page_size ?? items.length,
  }
}

export const http = {
  get: <T>(url: string, config?: AxiosRequestConfig) =>
    request<T>({ ...config, method: 'GET', url }),
  post: <T>(url: string, data?: unknown, config?: AxiosRequestConfig) =>
    request<T>({ ...config, method: 'POST', url, data }),
  put: <T>(url: string, data?: unknown, config?: AxiosRequestConfig) =>
    request<T>({ ...config, method: 'PUT', url, data }),
  patch: <T>(url: string, data?: unknown, config?: AxiosRequestConfig) =>
    request<T>({ ...config, method: 'PATCH', url, data }),
  delete: <T>(url: string, config?: AxiosRequestConfig) =>
    request<T>({ ...config, method: 'DELETE', url }),
  getList: <T>(url: string, config?: AxiosRequestConfig) =>
    requestList<T>({ ...config, method: 'GET', url }),
}

/** 仅供单测访问内部实现（不在业务层使用）。 */
export const __testing = { normalizeError, performRefresh }
