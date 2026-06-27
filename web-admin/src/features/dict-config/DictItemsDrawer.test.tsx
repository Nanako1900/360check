import { describe, expect, it } from 'vitest'
import { http as msw } from 'msw'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { server } from '@/mocks/server'
import { db } from '@/mocks/db'
import type { DictType } from '@/shared/api/types'
import { renderWithProviders } from '@/test/renderWithProviders'
import { DictItemsDrawer } from './DictItemsDrawer'
import { DictItemFormModal } from './DictItemFormModal'

const BASE = '*/api/v1'

function findType(code: string): DictType {
  const type = db.dictTypes.find((t) => t.code === code)
  if (!type) throw new Error(`missing seed type ${code}`)
  return type
}

describe('DictItemsDrawer', () => {
  it('retire toggle calls PUT /dict/items/{id} with is_active=false', async () => {
    let capturedBody: unknown = null
    server.use(
      msw.put(`${BASE}/dict/items/:id`, async ({ request }) => {
        capturedBody = await request.json()
        return Response.json({ success: true, data: null, error: null, meta: null })
      }),
    )

    const user = userEvent.setup()
    renderWithProviders(
      <DictItemsDrawer open dictType={findType('problem_status')} onClose={() => {}} />,
    )

    // 等待表格渲染出活动项 OPEN
    expect(await screen.findByText('待处理')).toBeInTheDocument()
    const retireButtons = await screen.findAllByRole('button', { name: '停用' })
    await user.click(retireButtons[0])
    // Popconfirm 确认
    const confirm = await screen.findByRole('button', { name: /确\s*定/ })
    await user.click(confirm)

    await waitFor(() => expect(capturedBody).toEqual({ is_active: false }))
  })

  it('renders retired items with an inactive tag (history tolerance)', async () => {
    renderWithProviders(
      <DictItemsDrawer open dictType={findType('problem_status')} onClose={() => {}} />,
    )
    // 退役项 LEGACY「历史状态」仍渲染
    expect(await screen.findByText('历史状态')).toBeInTheDocument()
    expect(screen.getAllByText('停用').length).toBeGreaterThan(0)
  })
})

describe('DictItemFormModal capture_preset extra', () => {
  it('rejects malformed extra jsonb via zod (quality out of range)', async () => {
    const user = userEvent.setup()
    renderWithProviders(
      <DictItemFormModal open dictType={findType('capture_main')} onClose={() => {}} />,
    )
    await user.type(screen.getByLabelText('机器键'), 'LOWQ')
    await user.type(screen.getByLabelText('显示名'), '低质量')

    const dialog = screen.getByRole('dialog')
    const extra = within(dialog).getByLabelText('扩展配置（jsonb）')
    await user.clear(extra)
    await user.type(extra, '{{"width":1024,"quality":500}')

    await user.click(within(dialog).getByRole('button', { name: /确\s*定/ }))
    // zod 拒绝 quality=500（>100），表单回填结构化错误
    expect(await screen.findByText(/quality：/)).toBeInTheDocument()
  })

  it('accepts valid extra jsonb and posts it', async () => {
    let capturedBody: unknown = null
    server.use(
      msw.post(`${BASE}/dict/items`, async ({ request }) => {
        capturedBody = await request.json()
        return Response.json(
          { success: true, data: { id: 999 }, error: null, meta: null },
          { status: 201 },
        )
      }),
    )
    const user = userEvent.setup()
    renderWithProviders(
      <DictItemFormModal open dictType={findType('capture_main')} onClose={() => {}} />,
    )
    await user.type(screen.getByLabelText('机器键'), 'GOODQ')
    await user.type(screen.getByLabelText('显示名'), '合格')

    const dialog = screen.getByRole('dialog')
    const extra = within(dialog).getByLabelText('扩展配置（jsonb）')
    await user.clear(extra)
    await user.type(extra, '{{"width":2048,"quality":90}')

    await user.click(within(dialog).getByRole('button', { name: /确\s*定/ }))
    await waitFor(() =>
      expect(capturedBody).toMatchObject({
        code: 'GOODQ',
        label: '合格',
        extra: { width: 2048, quality: 90 },
      }),
    )
  })
})
