import { afterEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { server } from '@/mocks/server'
import { EXPORT_RESULT_URL } from '@/mocks/db'
import { renderWithProviders } from '@/test/renderWithProviders'
import { ExportProgress } from './ExportProgress'
import * as download from './download'

const BASE = '*/api/v1'

describe('ExportProgress', () => {
  afterEach(() => vi.restoreAllMocks())

  it('renders progress rows and percent from the SSE stream', async () => {
    vi.spyOn(download, 'triggerDownload').mockImplementation(() => {})
    renderWithProviders(<ExportProgress jobUuid="job-sse" open onClose={vi.fn()} />)
    // 默认 SSE 流先发 progress(40,80) 再 done SUCCEEDED；断言出现「已处理 x / 100 行」。
    await waitFor(
      () => {
        const region = screen.getByTestId('export-progress')
        expect(region.getAttribute('data-status')).toBe('SUCCEEDED')
      },
      { timeout: 5000 },
    )
    expect(screen.getByText('导出完成，文件已开始下载。')).toBeInTheDocument()
  })

  it('shows the error message on a FAILED done frame', async () => {
    server.use(
      http.get(`${BASE}/exports/:jobUuid/events`, () => {
        const encoder = new TextEncoder()
        const stream = new ReadableStream<Uint8Array>({
          start(controller) {
            controller.enqueue(
              encoder.encode(
                'event: progress\ndata: {"progress":30,"processed_rows":30,"total_rows":100,"status":"RUNNING"}\n\n',
              ),
            )
            controller.enqueue(
              encoder.encode('event: done\ndata: {"status":"FAILED","error":"模板渲染失败"}\n\n'),
            )
            controller.close()
          },
        })
        return new HttpResponse(stream, {
          status: 200,
          headers: { 'Content-Type': 'text/event-stream' },
        })
      }),
    )
    renderWithProviders(<ExportProgress jobUuid="job-fail" open onClose={vi.fn()} />)
    expect(await screen.findByTestId('export-error')).toBeInTheDocument()
    expect(screen.getByText('模板渲染失败')).toBeInTheDocument()
  })

  it('re-fetches a fresh result_url when the signed link expired', async () => {
    const spy = vi.spyOn(download, 'triggerDownload').mockImplementation(() => {})
    renderWithProviders(<ExportProgress jobUuid="job-refresh" open onClose={vi.fn()} />)
    // 等到 SUCCEEDED + 自动下载按钮出现。
    const refreshBtn = await screen.findByText('重新获取下载链接', undefined, { timeout: 5000 })
    spy.mockClear()

    // 点「重新获取下载链接」→ GET /exports/:uuid 返回新 result_url → 再次触发下载。
    server.use(
      http.get(`${BASE}/exports/:jobUuid`, () =>
        HttpResponse.json({
          success: true,
          data: {
            id: 1,
            job_uuid: 'job-refresh',
            type: 'PROBLEM_LIST',
            status: 'SUCCEEDED',
            progress: 100,
            processed_rows: 100,
            total_rows: 100,
            result_url: 'https://cdn.example.com/exports/fresh.xlsx?sig=new',
            error: null,
            created_at: '2026-06-26T00:00:00Z',
          },
          error: null,
          meta: null,
        }),
      ),
    )
    const user = userEvent.setup()
    await user.click(refreshBtn)
    await waitFor(() =>
      expect(spy).toHaveBeenCalledWith('https://cdn.example.com/exports/fresh.xlsx?sig=new'),
    )
  })

  it('renders nothing actionable when closed (open=false)', () => {
    renderWithProviders(<ExportProgress jobUuid="job-x" open={false} onClose={vi.fn()} />)
    expect(screen.queryByTestId('export-progress')).toBeNull()
  })

  it('auto-downloads the result_url exactly once on SUCCEEDED', async () => {
    const spy = vi.spyOn(download, 'triggerDownload').mockImplementation(() => {})
    renderWithProviders(<ExportProgress jobUuid="job-once" open onClose={vi.fn()} />)
    await waitFor(() => expect(spy).toHaveBeenCalledWith(EXPORT_RESULT_URL), { timeout: 5000 })
    // 一段时间内不应重复触发自动下载。
    const callsAfterSuccess = spy.mock.calls.length
    await new Promise((r) => setTimeout(r, 50))
    expect(spy.mock.calls.length).toBe(callsAfterSuccess)
  })
})
