import { describe, expect, it, vi } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { server } from '@/mocks/server'
import { renderWithProviders } from '@/test/renderWithProviders'
import type { ProblemLogCreate } from '@/shared/api/types'
import { AddProcessingLogModal } from './AddProcessingLogModal'

const BASE = '*/api/v1'

describe('AddProcessingLogModal — D3: COMMENT/REASSIGN only, never STATUS_CHANGE', () => {
  it('posts a COMMENT log with the typed note (action=COMMENT)', async () => {
    const user = userEvent.setup()
    let captured: ProblemLogCreate | null = null
    server.use(
      http.post(`${BASE}/problems/:id/logs`, async ({ request }) => {
        captured = (await request.json()) as ProblemLogCreate
        return HttpResponse.json(
          { success: true, data: { id: 9, problem_id: 5001, action: 'COMMENT', created_at: 'x' }, error: null, meta: null },
          { status: 201 },
        )
      }),
    )
    renderWithProviders(<AddProcessingLogModal open problemId={5001} onClose={vi.fn()} />)

    const note = await screen.findByPlaceholderText('填写处理说明（改派可选）')
    await user.type(note, '已现场复核')
    await user.click(screen.getByRole('button', { name: /确\s*定/ }))

    await waitFor(() => expect(captured).not.toBeNull())
    expect(captured!.action).toBe('COMMENT')
    expect(captured!.note).toBe('已现场复核')
    // D3：前端绝不构造 STATUS_CHANGE。
    expect(captured!.action).not.toBe('STATUS_CHANGE')
  })

  it('never offers STATUS_CHANGE as a selectable action (only 备注 / 改派)', async () => {
    const user = userEvent.setup()
    renderWithProviders(<AddProcessingLogModal open problemId={5001} onClose={vi.fn()} />)
    // 打开动作下拉。
    const actionSelect = (await screen.findAllByRole('combobox'))[0]
    await user.click(actionSelect)
    // 下拉项仅「备注」「改派」，绝无「状态变更」。
    await waitFor(() => expect(screen.getAllByText('备注').length).toBeGreaterThan(0))
    expect(screen.getByText('改派')).toBeInTheDocument()
    expect(screen.queryByText('状态变更')).toBeNull()
  })

  it('posts a REASSIGN log with operator_id (new assignee), action=REASSIGN', async () => {
    const user = userEvent.setup()
    let captured: ProblemLogCreate | null = null
    server.use(
      http.post(`${BASE}/problems/:id/logs`, async ({ request }) => {
        captured = (await request.json()) as ProblemLogCreate
        return HttpResponse.json(
          { success: true, data: { id: 10, problem_id: 5001, action: 'REASSIGN', created_at: 'x' }, error: null, meta: null },
          { status: 201 },
        )
      }),
    )
    renderWithProviders(<AddProcessingLogModal open problemId={5001} onClose={vi.fn()} />)

    // 选 REASSIGN 动作。
    const actionSelect = (await screen.findAllByRole('combobox'))[0]
    await user.click(actionSelect)
    await user.click(await screen.findByText('改派'))

    // 选新负责人（改派给字段在 action=REASSIGN 时出现）。
    const assigneeLabel = await screen.findByText('改派给')
    const assigneeItem = assigneeLabel.closest('.ant-form-item') as HTMLElement
    const assignee = within(assigneeItem).getByRole('combobox')
    await user.click(assignee)
    await user.click(await screen.findByText('李巡查'))

    await user.click(screen.getByRole('button', { name: /确\s*定/ }))

    await waitFor(() => expect(captured).not.toBeNull())
    expect(captured!.action).toBe('REASSIGN')
    expect(captured!.operator_id).toBe(2)
  })
})
