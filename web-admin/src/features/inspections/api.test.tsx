import { describe, expect, it } from 'vitest'
import type { ReactNode } from 'react'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/mocks/server'
import { useInspection, useInspections, useTrajectory } from './api'

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

describe('inspections api', () => {
  it('builds the query string from all filters (project_id/inspector_id/status/from/to)', async () => {
    let captured = ''
    server.use(
      http.get(`${BASE}/inspections`, ({ request }) => {
        captured = new URL(request.url).search
        return HttpResponse.json({
          success: true,
          data: [],
          error: null,
          meta: { total: 0, page: 1, page_size: 20 },
        })
      }),
    )
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(
      () =>
        useInspections({
          project_id: 1,
          inspector_id: 2,
          status: 'FINISHED',
          from: '2026-06-01T00:00:00.000Z',
          to: '2026-06-30T00:00:00.000Z',
        }),
      { wrapper: Wrapper },
    )
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const params = new URLSearchParams(captured)
    expect(params.get('project_id')).toBe('1')
    expect(params.get('inspector_id')).toBe('2')
    expect(params.get('status')).toBe('FINISHED')
    expect(params.get('from')).toBe('2026-06-01T00:00:00.000Z')
    expect(params.get('to')).toBe('2026-06-30T00:00:00.000Z')
  })

  it('omits empty filter params from the query string', async () => {
    let captured = ''
    server.use(
      http.get(`${BASE}/inspections`, ({ request }) => {
        captured = new URL(request.url).search
        return HttpResponse.json({
          success: true,
          data: [],
          error: null,
          meta: { total: 0, page: 1, page_size: 20 },
        })
      }),
    )
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useInspections({ project_id: 1 }), { wrapper: Wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const params = new URLSearchParams(captured)
    expect(params.get('project_id')).toBe('1')
    expect(params.has('inspector_id')).toBe(false)
    expect(params.has('status')).toBe(false)
    expect(params.has('from')).toBe(false)
    expect(params.has('to')).toBe(false)
  })

  it('useInspections filters seeded data by project and status', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(
      () => useInspections({ project_id: 1, status: 'FINISHED' }),
      { wrapper: Wrapper },
    )
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.items).toHaveLength(1)
    expect(result.current.data?.items[0].status).toBe('FINISHED')
  })

  it('useInspection reads a single record including null route_geom for in-progress', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useInspection(2), { wrapper: Wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.status).toBe('IN_PROGRESS')
    expect(result.current.data?.route_geom).toBeUndefined()
  })

  it('useTrajectory returns a WGS84 LineString + points for the finished inspection', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useTrajectory(1), { wrapper: Wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.route?.type).toBe('LineString')
    expect((result.current.data?.route?.coordinates.length ?? 0)).toBeGreaterThanOrEqual(2)
    expect(result.current.data?.points.length ?? 0).toBeGreaterThan(0)
  })

  it('useTrajectory returns a null route + empty points for an in-progress inspection', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useTrajectory(2), { wrapper: Wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.route).toBeUndefined()
    expect(result.current.data?.points).toEqual([])
  })
})
