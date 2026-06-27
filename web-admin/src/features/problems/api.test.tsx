import { describe, expect, it } from 'vitest'
import type { ReactNode } from 'react'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/mocks/server'
import { ApiError } from '@/shared/api/apiError'
import type { ProblemLogCreate } from '@/shared/api/types'
import {
  useAppendProblemLog,
  useDeleteProblem,
  useProblem,
  useProblemLogs,
  useProblems,
  useProblemsMap,
  useUpdateProblem,
} from './api'

const BASE = '*/api/v1'

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { retry: false } },
  })
  function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
  return { qc, Wrapper }
}

describe('problems map api', () => {
  it('returns a FeatureCollection of WGS84 points for the seeded inspection', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useProblemsMap({ inspection_id: 1 }), { wrapper: Wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.type).toBe('FeatureCollection')
    expect(result.current.data?.features.length ?? 0).toBeGreaterThanOrEqual(2)
    const first = result.current.data?.features[0]
    expect(first?.geometry.type).toBe('Point')
    expect(first?.properties.status_label).toBeTruthy()
  })

  it('honors the inspection_id filter (empty for an inspection with no problems)', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useProblemsMap({ inspection_id: 2 }), { wrapper: Wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.features).toEqual([])
  })

  it('builds the query string from all provided filters', async () => {
    let captured = ''
    server.use(
      http.get(`${BASE}/problems/map`, ({ request }) => {
        captured = new URL(request.url).search
        return HttpResponse.json({
          success: true,
          data: { type: 'FeatureCollection', features: [] },
          error: null,
          meta: null,
        })
      }),
    )
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(
      () =>
        useProblemsMap({
          project_id: 1,
          inspection_id: 1,
          type: 201,
          status: 101,
          category: 9,
          inspector_id: 2,
          from: '2026-06-01T00:00:00.000Z',
          to: '2026-06-30T00:00:00.000Z',
          limit: 500,
        }),
      { wrapper: Wrapper },
    )
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const params = new URLSearchParams(captured)
    expect(params.get('project_id')).toBe('1')
    expect(params.get('inspection_id')).toBe('1')
    expect(params.get('type')).toBe('201')
    expect(params.get('status')).toBe('101')
    expect(params.get('category')).toBe('9')
    expect(params.get('inspector_id')).toBe('2')
    expect(params.get('from')).toBe('2026-06-01T00:00:00.000Z')
    expect(params.get('to')).toBe('2026-06-30T00:00:00.000Z')
    expect(params.get('limit')).toBe('500')
  })

  it('filters by project_id against the seeded data', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useProblemsMap({ project_id: 999 }), { wrapper: Wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.features).toEqual([])
  })
})

describe('useProblems list api', () => {
  it('builds the query string from all 8 filters incl inspection_id and category', async () => {
    let captured = ''
    server.use(
      http.get(`${BASE}/problems`, ({ request }) => {
        captured = new URL(request.url).search
        return HttpResponse.json({
          success: true,
          data: [],
          error: null,
          meta: { total: 0, page: 1, page_size: 10 },
        })
      }),
    )
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(
      () =>
        useProblems({
          project_id: 1,
          type: 201,
          status: 101,
          category: 401,
          inspector_id: 2,
          inspection_id: 1,
          from: '2026-06-01T00:00:00.000Z',
          to: '2026-06-30T00:00:00.000Z',
          page: 1,
          page_size: 10,
        }),
      { wrapper: Wrapper },
    )
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const params = new URLSearchParams(captured)
    expect(params.get('project_id')).toBe('1')
    expect(params.get('type')).toBe('201')
    expect(params.get('status')).toBe('101')
    expect(params.get('category')).toBe('401')
    expect(params.get('inspector_id')).toBe('2')
    expect(params.get('inspection_id')).toBe('1')
    expect(params.get('from')).toBe('2026-06-01T00:00:00.000Z')
    expect(params.get('to')).toBe('2026-06-30T00:00:00.000Z')
    expect(params.get('page')).toBe('1')
    expect(params.get('page_size')).toBe('10')
  })

  it('returns seeded problems and honors the inspection_id filter (D1)', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useProblems({ inspection_id: 1 }), { wrapper: Wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.items.length).toBe(2)
    expect(result.current.data?.items.every((p) => p.inspection_id === 1)).toBe(true)
  })

  it('keeps a problem referencing a RETIRED status dict item (history tolerance)', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useProblem(5003), { wrapper: Wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    // 退役状态项 id=199 仍随问题返回（渲染层由 DictTag 历史容忍）。
    expect(result.current.data?.status_item_id).toBe(199)
  })
})

describe('useUpdateProblem — D3 single PUT, no STATUS_CHANGE POST', () => {
  it('sends exactly ONE PUT and NO logs POST on status change; logs refetch shows backend STATUS_CHANGE', async () => {
    let putCount = 0
    let logsPostCount = 0
    server.use(
      http.put(`${BASE}/problems/:id`, async ({ request, params }) => {
        putCount += 1
        const body = (await request.json()) as { status_item_id?: number }
        return HttpResponse.json({
          success: true,
          error: null,
          meta: null,
          data: {
            id: Number(params.id),
            client_uuid: 'x',
            project_id: 1,
            inspection_id: 1,
            inspector_id: 2,
            geom: { type: 'Point', coordinates: [120.21, 30.25] },
            status_item_id: body.status_item_id ?? 101,
            dict_version_used: 1,
            captured_at: '2026-06-20T01:20:00Z',
            created_at: '2026-06-26T00:00:00Z',
            updated_at: '2026-06-26T00:00:00Z',
          },
        })
      }),
      http.post(`${BASE}/problems/:id/logs`, () => {
        logsPostCount += 1
        return HttpResponse.json({ success: true, data: null, error: null, meta: null }, { status: 201 })
      }),
    )

    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useUpdateProblem(5001), { wrapper: Wrapper })
    await result.current.mutateAsync({ status_item_id: 102 })

    // D3：恰好一次 PUT，绝不 POST STATUS_CHANGE（实际上一次 logs POST 都没有）。
    expect(putCount).toBe(1)
    expect(logsPostCount).toBe(0)
  })

  it('end-to-end against the mock: PUT status then logs refetch contains a backend STATUS_CHANGE row', async () => {
    const { Wrapper } = makeWrapper()
    const update = renderHook(() => useUpdateProblem(5001), { wrapper: Wrapper })
    await update.result.current.mutateAsync({ status_item_id: 103 })

    const logs = renderHook(() => useProblemLogs(5001), { wrapper: Wrapper })
    await waitFor(() => expect(logs.result.current.isSuccess).toBe(true))
    const items = logs.result.current.data?.items ?? []
    const statusChange = items.find((l) => l.action === 'STATUS_CHANGE')
    expect(statusChange).toBeDefined()
    expect(statusChange?.from_status_item_id).toBe(101)
    expect(statusChange?.to_status_item_id).toBe(103)
  })
})

describe('useAppendProblemLog — COMMENT/REASSIGN only (D3)', () => {
  it('posts a COMMENT log', async () => {
    let captured: ProblemLogCreate | null = null
    server.use(
      http.post(`${BASE}/problems/:id/logs`, async ({ request }) => {
        captured = (await request.json()) as ProblemLogCreate
        return HttpResponse.json(
          { success: true, data: { id: 1, problem_id: 5001, action: 'COMMENT', created_at: 'x' }, error: null, meta: null },
          { status: 201 },
        )
      }),
    )
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useAppendProblemLog(5001), { wrapper: Wrapper })
    await result.current.mutateAsync({ action: 'COMMENT', note: '复核完成' })
    expect(captured).not.toBeNull()
    expect(captured!.action).toBe('COMMENT')
  })

  it('the mock backend rejects a STATUS_CHANGE POST with 422 (frontend never sends it)', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useAppendProblemLog(5001), { wrapper: Wrapper })
    // 强行构造 STATUS_CHANGE（绕过类型）以证明后端会拒绝；前端正常路径永不发此动作。
    await expect(
      result.current.mutateAsync({ action: 'STATUS_CHANGE' } as unknown as ProblemLogCreate),
    ).rejects.toBeInstanceOf(ApiError)
  })
})

describe('useDeleteProblem', () => {
  it('soft-deletes via DELETE /problems/{id}', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useDeleteProblem(), { wrapper: Wrapper })
    await result.current.mutateAsync(5002)
    const list = renderHook(() => useProblems({ inspection_id: 1 }), { wrapper: Wrapper })
    await waitFor(() => expect(list.result.current.isSuccess).toBe(true))
    expect(list.result.current.data?.items.some((p) => p.id === 5002)).toBe(false)
  })
})
