import { beforeEach, describe, expect, it, vi } from 'vitest'
import { http as msw, HttpResponse, delay } from 'msw'
import { server } from '@/mocks/server'
import { USERS } from '@/mocks/db'
import { useAuthStore } from '@/shared/auth/authStore'
import { sessionBus } from '@/shared/auth/sessionBus'
import { http } from './http'
import { ApiError } from './apiError'
import type { ApiErrorCode } from './apiError'

const BASE = '*/api/v1'
const user = USERS[0]

function ok<T>(data: T, meta: unknown = null) {
  return HttpResponse.json({ success: true, data, error: null, meta })
}
function fail(code: ApiErrorCode, message: string, status: number) {
  return HttpResponse.json(
    { success: false, data: null, error: { code, message, details: [] }, meta: null },
    { status },
  )
}
function seedTokens(access: string, refresh: string) {
  useAuthStore.getState().setTokens({
    access_token: access,
    refresh_token: refresh,
    expires_in: 900,
    user,
  })
}

beforeEach(() => {
  useAuthStore.getState().reset()
})

describe('http envelope unwrap', () => {
  it('unwraps a success envelope to data', async () => {
    server.use(msw.get(`${BASE}/thing`, () => ok({ id: 7 })))
    await expect(http.get('/thing')).resolves.toEqual({ id: 7 })
  })

  it('getList returns items + meta', async () => {
    server.use(
      msw.get(`${BASE}/things`, () =>
        ok([{ id: 1 }, { id: 2 }], { total: 42, page: 2, page_size: 10 }),
      ),
    )
    const res = await http.getList<{ id: number }>('/things')
    expect(res.items).toHaveLength(2)
    expect(res).toMatchObject({ total: 42, page: 2, pageSize: 10 })
  })

  it('throws a typed ApiError on an error envelope', async () => {
    server.use(msw.get(`${BASE}/boom`, () => fail('NOT_FOUND', '不存在', 404)))
    await expect(http.get('/boom')).rejects.toBeInstanceOf(ApiError)
    await expect(http.get('/boom')).rejects.toMatchObject({ code: 'NOT_FOUND', httpStatus: 404 })
  })

  it('normalizes a network error to INTERNAL', async () => {
    server.use(msw.get(`${BASE}/down`, () => HttpResponse.error()))
    await expect(http.get('/down')).rejects.toMatchObject({ code: 'INTERNAL' })
  })
})

describe('Bearer injection', () => {
  it('attaches the access token from the store', async () => {
    seedTokens('tk', 'r')
    let seen: string | null = null
    server.use(
      msw.get(`${BASE}/whoami`, ({ request }) => {
        seen = request.headers.get('Authorization')
        return ok({ ok: true })
      }),
    )
    await http.get('/whoami')
    expect(seen).toBe('Bearer tk')
  })
})

describe('401 single-flight refresh', () => {
  it('refreshes once on TOKEN_EXPIRED then replays transparently', async () => {
    seedTokens('old', 'r-old')
    let refreshCalls = 0
    server.use(
      msw.get(`${BASE}/secure`, ({ request }) =>
        request.headers.get('Authorization') === 'Bearer old'
          ? fail('TOKEN_EXPIRED', '过期', 401)
          : ok({ secure: true, tok: request.headers.get('Authorization') }),
      ),
      msw.post(`${BASE}/auth/refresh`, () => {
        refreshCalls += 1
        return ok({ access_token: 'new', refresh_token: 'r-new', expires_in: 900, user })
      }),
    )
    const res = await http.get<{ secure: boolean; tok: string }>('/secure')
    expect(res).toEqual({ secure: true, tok: 'Bearer new' })
    expect(refreshCalls).toBe(1)
    expect(useAuthStore.getState().accessToken).toBe('new')
    expect(useAuthStore.getState().refreshToken).toBe('r-new')
  })

  it('coalesces concurrent TOKEN_EXPIRED into a single refresh', async () => {
    seedTokens('old', 'r-old')
    let refreshCalls = 0
    const guarded = (tag: string) =>
      msw.get(`${BASE}/${tag}`, ({ request }) =>
        request.headers.get('Authorization') === 'Bearer old'
          ? fail('TOKEN_EXPIRED', '过期', 401)
          : ok({ tag }),
      )
    server.use(
      guarded('a'),
      guarded('b'),
      guarded('c'),
      msw.post(`${BASE}/auth/refresh`, async () => {
        refreshCalls += 1
        await delay(10)
        return ok({ access_token: 'new', refresh_token: 'r-new', expires_in: 900, user })
      }),
    )
    const results = await Promise.all([http.get('/a'), http.get('/b'), http.get('/c')])
    expect(results).toEqual([{ tag: 'a' }, { tag: 'b' }, { tag: 'c' }])
    expect(refreshCalls).toBe(1)
  })

  it('logs out when the refresh itself fails', async () => {
    seedTokens('old', 'bad')
    const onLogout = vi.fn()
    const off = sessionBus.onForceLogout(onLogout)
    server.use(
      msw.get(`${BASE}/secure`, () => fail('TOKEN_EXPIRED', '过期', 401)),
      msw.post(`${BASE}/auth/refresh`, () => fail('UNAUTHENTICATED', '刷新失败', 401)),
    )
    await expect(http.get('/secure')).rejects.toBeInstanceOf(ApiError)
    expect(useAuthStore.getState().accessToken).toBeNull()
    expect(onLogout).toHaveBeenCalledWith('REFRESH_FAILED')
    off()
  })

  it('logs out when the replayed request is still TOKEN_EXPIRED after a successful refresh', async () => {
    seedTokens('old', 'r-old')
    let refreshCalls = 0
    const onLogout = vi.fn()
    const off = sessionBus.onForceLogout(onLogout)
    server.use(
      // 始终返回 TOKEN_EXPIRED：即使刷新成功，重放仍过期（时钟漂移/后端撤销等）
      msw.get(`${BASE}/secure`, () => fail('TOKEN_EXPIRED', '过期', 401)),
      msw.post(`${BASE}/auth/refresh`, () => {
        refreshCalls += 1
        return ok({ access_token: 'new', refresh_token: 'r-new', expires_in: 900, user })
      }),
    )
    await expect(http.get('/secure')).rejects.toMatchObject({ code: 'TOKEN_EXPIRED' })
    expect(refreshCalls).toBe(1) // 只刷新一次（_retried 守卫）
    expect(useAuthStore.getState().accessToken).toBeNull()
    expect(onLogout).toHaveBeenCalledWith('UNAUTHENTICATED')
    off()
  })
})

describe('401 UNAUTHENTICATED', () => {
  it('resets the session and emits force-logout without refreshing', async () => {
    seedTokens('old', 'r')
    let refreshCalls = 0
    const onLogout = vi.fn()
    const off = sessionBus.onForceLogout(onLogout)
    server.use(
      msw.get(`${BASE}/secure`, () => fail('UNAUTHENTICATED', '未认证', 401)),
      msw.post(`${BASE}/auth/refresh`, () => {
        refreshCalls += 1
        return ok({ access_token: 'x', refresh_token: 'y', expires_in: 1, user })
      }),
    )
    await expect(http.get('/secure')).rejects.toMatchObject({ code: 'UNAUTHENTICATED' })
    expect(refreshCalls).toBe(0)
    expect(useAuthStore.getState().accessToken).toBeNull()
    expect(onLogout).toHaveBeenCalledWith('UNAUTHENTICATED')
    off()
  })
})
