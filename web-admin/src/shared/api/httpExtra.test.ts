import { beforeEach, describe, expect, it } from 'vitest'
import { http as msw, HttpResponse } from 'msw'
import type { AxiosError } from 'axios'
import { server } from '@/mocks/server'
import { useAuthStore } from '@/shared/auth/authStore'
import { http, __testing } from './http'
import { ApiError } from './apiError'
import type { Envelope } from './types'

const BASE = '*/api/v1'

beforeEach(() => {
  useAuthStore.getState().reset()
})

describe('http verbs', () => {
  it('put / patch / delete reach the right method + unwrap', async () => {
    server.use(
      msw.put(`${BASE}/r/1`, async ({ request }) =>
        HttpResponse.json({
          success: true,
          data: { m: 'PUT', body: await request.json() },
          error: null,
          meta: null,
        }),
      ),
      msw.patch(`${BASE}/r/1`, () =>
        HttpResponse.json({ success: true, data: { m: 'PATCH' }, error: null, meta: null }),
      ),
      msw.delete(`${BASE}/r/1`, () =>
        HttpResponse.json({ success: true, data: { m: 'DELETE' }, error: null, meta: null }),
      ),
    )
    await expect(http.put('/r/1', { x: 1 })).resolves.toEqual({ m: 'PUT', body: { x: 1 } })
    await expect(http.patch('/r/1', { y: 2 })).resolves.toEqual({ m: 'PATCH' })
    await expect(http.delete('/r/1')).resolves.toEqual({ m: 'DELETE' })
  })
})

describe('envelope success:false on 2xx (belt-and-suspenders)', () => {
  it('request throws ApiError when body.success is false despite HTTP 200', async () => {
    server.use(
      msw.get(`${BASE}/weird`, () =>
        HttpResponse.json(
          {
            success: false,
            data: null,
            error: { code: 'CONFLICT', message: '冲突', details: [] },
            meta: null,
          },
          { status: 200 },
        ),
      ),
    )
    await expect(http.get('/weird')).rejects.toMatchObject({ code: 'CONFLICT' })
  })

  it('getList throws ApiError when body.success is false despite HTTP 200', async () => {
    server.use(
      msw.get(`${BASE}/weird-list`, () =>
        HttpResponse.json(
          {
            success: false,
            data: null,
            error: { code: 'INTERNAL', message: 'x', details: [] },
            meta: null,
          },
          { status: 200 },
        ),
      ),
    )
    await expect(http.getList('/weird-list')).rejects.toBeInstanceOf(ApiError)
  })

  it('getList defaults meta from item count when meta is absent', async () => {
    server.use(
      msw.get(`${BASE}/nometa`, () =>
        HttpResponse.json({ success: true, data: [{ id: 1 }], error: null, meta: null }),
      ),
    )
    const res = await http.getList('/nometa')
    expect(res).toMatchObject({ total: 1, page: 1, pageSize: 1 })
  })
})

describe('normalizeError', () => {
  function asAxios(partial: Partial<AxiosError<Envelope<unknown>>>): AxiosError<Envelope<unknown>> {
    return partial as AxiosError<Envelope<unknown>>
  }

  it('maps an envelope error.code to ApiError', () => {
    const e = __testing.normalizeError(
      asAxios({
        response: {
          status: 409,
          data: {
            success: false,
            data: null,
            error: { code: 'CONFLICT', message: 'c', details: [] },
            meta: null,
          },
        } as unknown as AxiosError<Envelope<unknown>>['response'],
      }),
    )
    expect(e).toMatchObject({ code: 'CONFLICT', httpStatus: 409 })
  })

  it('maps a timeout (ECONNABORTED) to INTERNAL', () => {
    const e = __testing.normalizeError(asAxios({ code: 'ECONNABORTED', message: 'timeout' }))
    expect(e.code).toBe('INTERNAL')
    expect(e.message).toContain('超时')
  })

  it('maps a bare network error to INTERNAL', () => {
    const e = __testing.normalizeError(asAxios({ message: 'Network Error' }))
    expect(e.code).toBe('INTERNAL')
  })
})
