import { describe, expect, it } from 'vitest'
import type { ReactNode } from 'react'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/mocks/server'
import { db } from '@/mocks/db'
import type { ExportJobCreate } from '@/shared/api/types'
import { useCreateExport, useExportJob } from './api'

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

describe('exports api', () => {
  it('useCreateExport POSTs type + params and returns a PENDING job', async () => {
    let captured: ExportJobCreate | null = null
    server.use(
      http.post(`${BASE}/exports`, async ({ request }) => {
        captured = (await request.json()) as ExportJobCreate
        return HttpResponse.json(
          {
            success: true,
            data: {
              id: 1,
              job_uuid: 'job-test',
              type: captured.type,
              params: captured.params,
              status: 'PENDING',
              progress: 0,
              processed_rows: 0,
              total_rows: 100,
              result_url: null,
              error: null,
              created_at: '2026-06-26T00:00:00Z',
            },
            error: null,
            meta: null,
          },
          { status: 201 },
        )
      }),
    )
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useCreateExport(), { wrapper: Wrapper })

    result.current.mutate({ type: 'PROBLEM_LIST', params: { project_id: 7, status: 5 } })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(captured).toEqual({ type: 'PROBLEM_LIST', params: { project_id: 7, status: 5 } })
    expect(result.current.data?.job_uuid).toBe('job-test')
    expect(result.current.data?.status).toBe('PENDING')
  })

  it('useCreateExport against the default handler stores a job keyed by job_uuid', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useCreateExport(), { wrapper: Wrapper })
    result.current.mutate({ type: 'INSPECTION_RECORDS', params: { project_id: 1 } })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const uuid = result.current.data?.job_uuid as string
    expect(db.exportJobs.get(uuid)?.type).toBe('INSPECTION_RECORDS')
  })

  it('useExportJob polls a single job and advances its progress', async () => {
    const { Wrapper } = makeWrapper()
    // 先创建一个 job。
    const { result: createResult } = renderHook(() => useCreateExport(), { wrapper: Wrapper })
    createResult.current.mutate({ type: 'PROJECT_STATS', params: {} })
    await waitFor(() => expect(createResult.current.isSuccess).toBe(true))
    const uuid = createResult.current.data?.job_uuid as string

    const { result } = renderHook(() => useExportJob(uuid), { wrapper: Wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.job_uuid).toBe(uuid)
    // 第一次 GET 即推进出 RUNNING（progress 增长）或 SUCCEEDED。
    expect(result.current.data?.progress ?? 0).toBeGreaterThan(0)
  })

  it('useExportJob is disabled when jobUuid is null', () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useExportJob(null), { wrapper: Wrapper })
    expect(result.current.fetchStatus).toBe('idle')
  })
})
