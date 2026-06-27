import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { server } from '@/mocks/server'
import { db, EXPORT_RESULT_URL, USERS } from '@/mocks/db'
import { useAuthStore } from '@/shared/auth/authStore'
import { renderWithProviders } from '@/test/renderWithProviders'
import { ExportButton } from './ExportButton'
import * as download from './download'

const BASE = '*/api/v1'

function grant(perms: string[]): void {
  useAuthStore.getState().setMe({ user: USERS[0], roles: [], permissions: perms })
}

describe('ExportButton', () => {
  beforeEach(() => {
    grant(['export:create', 'export:read'])
  })
  afterEach(() => {
    useAuthStore.getState().reset()
    vi.restoreAllMocks()
  })

  it('is disabled without export:create permission (UI gating)', () => {
    grant(['export:read'])
    renderWithProviders(<ExportButton type="PROBLEM_LIST" params={{}} />)
    expect(screen.getByTestId('export-button-PROBLEM_LIST')).toBeDisabled()
  })

  it('POSTs the type + current filter params (所见即所导) on click', async () => {
    let captured: { type: string; params?: Record<string, unknown> } | null = null
    server.use(
      http.post(`${BASE}/exports`, async ({ request }) => {
        captured = (await request.json()) as { type: string; params?: Record<string, unknown> }
        return HttpResponse.json(
          {
            success: true,
            data: {
              id: 9,
              job_uuid: 'job-click',
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
      // 让进度弹层走轮询直接出 SUCCEEDED（避免依赖 SSE 流时序）。
      http.get(`${BASE}/exports/:jobUuid`, () =>
        HttpResponse.json({
          success: true,
          data: {
            id: 9,
            job_uuid: 'job-click',
            type: 'PROBLEM_LIST',
            status: 'SUCCEEDED',
            progress: 100,
            processed_rows: 100,
            total_rows: 100,
            result_url: EXPORT_RESULT_URL,
            error: null,
            created_at: '2026-06-26T00:00:00Z',
          },
          error: null,
          meta: null,
        }),
      ),
      http.get(`${BASE}/exports/:jobUuid/events`, () => new HttpResponse(null, { status: 500 })),
    )

    const user = userEvent.setup()
    renderWithProviders(
      <ExportButton
        type="PROBLEM_LIST"
        params={{ project_id: 3, status: 5, inspector_id: undefined, from: '' }}
      />,
    )
    await user.click(screen.getByTestId('export-button-PROBLEM_LIST'))

    await waitFor(() => expect(captured).not.toBeNull())
    // undefined / '' 被裁剪掉，只保留有效筛选。
    expect(captured).toEqual({ type: 'PROBLEM_LIST', params: { project_id: 3, status: 5 } })
  })

  it('opens the progress modal and auto-downloads on SUCCEEDED (polling path)', async () => {
    const spy = vi.spyOn(download, 'triggerDownload').mockImplementation(() => {})
    // 强制 SSE 失败 → 走轮询回退（默认 GET handler 推进到 SUCCEEDED）。
    server.use(
      http.get(`${BASE}/exports/:jobUuid/events`, () => new HttpResponse(null, { status: 500 })),
    )

    const user = userEvent.setup()
    renderWithProviders(
      <ExportButton type="INSPECTION_RECORDS" params={{ project_id: 1 }} pollIntervalMs={20} />,
    )
    await user.click(screen.getByTestId('export-button-INSPECTION_RECORDS'))

    // 进度弹层出现。
    const progress = await screen.findByTestId('export-progress')
    expect(progress).toBeInTheDocument()

    // 轮询推进到 SUCCEEDED → 自动下载被调用（带 result_url）。
    await waitFor(() => expect(spy).toHaveBeenCalledWith(EXPORT_RESULT_URL), { timeout: 5000 })
    expect(await screen.findByTestId('export-download-button')).toBeInTheDocument()
  })

  it('shows the polling fallback notice when SSE drops', async () => {
    vi.spyOn(download, 'triggerDownload').mockImplementation(() => {})
    server.use(
      http.get(`${BASE}/exports/:jobUuid/events`, () => new HttpResponse(null, { status: 500 })),
    )
    const user = userEvent.setup()
    renderWithProviders(<ExportButton type="PROJECT_STATS" params={{}} />)
    await user.click(screen.getByTestId('export-button-PROJECT_STATS'))
    expect(await screen.findByTestId('export-polling')).toBeInTheDocument()
  })

  it('surfaces a FORBIDDEN error from the backend (perms are backend-enforced)', async () => {
    server.use(
      http.post(`${BASE}/exports`, () =>
        HttpResponse.json(
          {
            success: false,
            data: null,
            error: { code: 'FORBIDDEN', message: 'no', details: [] },
            meta: null,
          },
          { status: 403 },
        ),
      ),
    )
    const user = userEvent.setup()
    renderWithProviders(<ExportButton type="PROBLEM_LIST" params={{}} />)
    await user.click(screen.getByTestId('export-button-PROBLEM_LIST'))
    expect(await screen.findByText('没有权限执行导出操作。')).toBeInTheDocument()
  })

  it('records the created job in the mock db with its params', async () => {
    const user = userEvent.setup()
    server.use(
      http.get(`${BASE}/exports/:jobUuid/events`, () => new HttpResponse(null, { status: 500 })),
    )
    renderWithProviders(<ExportButton type="PROJECT_STATS" params={{ project_id: 2 }} />)
    await user.click(screen.getByTestId('export-button-PROJECT_STATS'))
    await screen.findByTestId('export-progress')
    const created = [...db.exportJobs.values()].find((j) => j.type === 'PROJECT_STATS')
    expect(created?.params).toEqual({ project_id: 2 })
  })

  it('renders a manual download + result alert region on success (SSE path)', async () => {
    vi.spyOn(download, 'triggerDownload').mockImplementation(() => {})
    // 不覆盖 events handler → 走默认 SSE 流（progress×2 + done SUCCEEDED）。
    const user = userEvent.setup()
    renderWithProviders(<ExportButton type="INSPECTION_RECORDS" params={{}} />)
    await user.click(screen.getByTestId('export-button-INSPECTION_RECORDS'))
    const dialog = await screen.findByRole('dialog')
    await within(dialog).findByTestId('export-download-button', undefined, { timeout: 5000 })
    expect(await screen.findByText('导出完成，文件已开始下载。')).toBeInTheDocument()
  })
})
