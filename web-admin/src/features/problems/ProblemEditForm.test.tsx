import { describe, expect, it, beforeEach } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { server } from '@/mocks/server'
import { renderWithProviders } from '@/test/renderWithProviders'
import { clearDictCache } from '@/shared/dict/dictCache'
import type { Problem } from '@/shared/api/types'
import { ProblemEditForm } from './ProblemEditForm'
import { ProblemProcessingLog } from './ProblemProcessingLog'

const BASE = '*/api/v1'

const PROBLEM: Problem = {
  id: 5001,
  client_uuid: 'dddddddd-0000-0000-0000-000000005001',
  project_id: 1,
  inspection_id: 1,
  inspector_id: 2,
  geom: { type: 'Point', coordinates: [120.2115, 30.2509] },
  type_item_id: 201,
  status_item_id: 101,
  category_item_id: 401,
  dict_version_used: 1,
  title: '步道路面横向裂缝，长约 1.2m',
  description: '描述',
  note: '已现场拍照',
  captured_at: '2026-06-20T01:20:00Z',
  cover_media_id: 7101,
  created_at: '2026-06-26T00:00:00Z',
  updated_at: '2026-06-26T00:00:00Z',
}

describe('ProblemEditForm — D3: status change = single PUT, never a STATUS_CHANGE POST', () => {
  beforeEach(() => clearDictCache())

  it('sends exactly ONE PUT and ZERO logs POST when changing status', async () => {
    const user = userEvent.setup()
    let putCount = 0
    let logsPostCount = 0
    let capturedStatus: number | null = null
    server.use(
      http.put(`${BASE}/problems/:id`, async ({ request, params }) => {
        putCount += 1
        const body = (await request.json()) as { status_item_id?: number }
        capturedStatus = body.status_item_id ?? null
        return HttpResponse.json({
          success: true,
          error: null,
          meta: null,
          data: { ...PROBLEM, id: Number(params.id), status_item_id: body.status_item_id ?? 101 },
        })
      }),
      http.post(`${BASE}/problems/:id/logs`, () => {
        logsPostCount += 1
        return HttpResponse.json({ success: true, data: null, error: null, meta: null }, { status: 201 })
      }),
    )

    renderWithProviders(<ProblemEditForm problem={PROBLEM} canUpdate />)

    // 改状态下拉到「处理中」(102)。
    const statusField = await screen.findByText('状态')
    const formItem = statusField.closest('.ant-form-item') as HTMLElement
    const combo = within(formItem).getByRole('combobox')
    await user.click(combo)
    await user.click(await screen.findByText('处理中'))

    await user.click(screen.getByRole('button', { name: /保\s*存/ }))

    await waitFor(() => expect(putCount).toBe(1))
    // D3 强约束：状态变更只发一次 PUT，绝不 POST STATUS_CHANGE（一次 logs POST 都没有）。
    expect(putCount).toBe(1)
    expect(logsPostCount).toBe(0)
    expect(capturedStatus).toBe(102)
  })

  it('after the PUT, a logs refetch surfaces the backend-generated STATUS_CHANGE row', async () => {
    const user = userEvent.setup()
    // 用默认 mock（会在 PUT 改状态时原子追加 STATUS_CHANGE）。
    const { rerender } = renderWithProviders(
      <>
        <ProblemEditForm problem={PROBLEM} canUpdate />
        <ProblemProcessingLog problemId={5001} />
      </>,
    )

    const statusField = await screen.findByText('状态')
    const formItem = statusField.closest('.ant-form-item') as HTMLElement
    const combo = within(formItem).getByRole('combobox')
    await user.click(combo)
    await user.click(await screen.findByText('已解决'))
    await user.click(screen.getByRole('button', { name: /保\s*存/ }))

    rerender(
      <>
        <ProblemEditForm problem={PROBLEM} canUpdate />
        <ProblemProcessingLog problemId={5001} />
      </>,
    )

    // 后端在 PUT 同一事务内写的 STATUS_CHANGE 行经 logs 刷新出现。
    expect(await screen.findByText('状态变更')).toBeInTheDocument()
  })

  it('disables the form when the user lacks problem:update', async () => {
    renderWithProviders(<ProblemEditForm problem={PROBLEM} canUpdate={false} />)
    const save = await screen.findByRole('button', { name: /保\s*存/ })
    expect(save).toBeDisabled()
  })
})
