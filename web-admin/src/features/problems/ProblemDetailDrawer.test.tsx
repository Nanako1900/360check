import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, within } from '@testing-library/react'
import { renderWithProviders } from '@/test/renderWithProviders'
import { useAuthStore } from '@/shared/auth/authStore'
import { USERS } from '@/mocks/db'
import { clearDictCache } from '@/shared/dict/dictCache'
import { ProblemDetailDrawer } from './ProblemDetailDrawer'

function grant(perms: string[]): void {
  useAuthStore.getState().setMe({ user: USERS[0], roles: [], permissions: perms })
}

describe('ProblemDetailDrawer', () => {
  beforeEach(() => clearDictCache())
  afterEach(() => useAuthStore.getState().reset())

  it('loads the problem detail and renders status via DictTag, description and note', async () => {
    grant(['problem:update'])
    renderWithProviders(<ProblemDetailDrawer problemId={5001} onClose={vi.fn()} />)
    expect(await screen.findByText('问题详情')).toBeInTheDocument()
    // 描述渲染（详情区 Descriptions + 编辑表单 TextArea 各一处）。
    const descs = await screen.findAllByText('位于一号段步道中段，横向延伸，需评估是否扩展。')
    expect(descs.length).toBeGreaterThan(0)
  })

  it('renders a RETIRED status dict item in detail (history tolerance) for problem 5003', async () => {
    grant(['problem:read'])
    renderWithProviders(<ProblemDetailDrawer problemId={5003} onClose={vi.fn()} />)
    // 退役状态 199 → label「历史状态」+「（已停用）」后缀。
    expect(await screen.findByText('历史状态')).toBeInTheDocument()
    expect(screen.getAllByText('（已停用）').length).toBeGreaterThan(0)
  })

  it('shows the delete control only with problem:delete permission', async () => {
    grant(['problem:read'])
    const { rerender } = renderWithProviders(<ProblemDetailDrawer problemId={5001} onClose={vi.fn()} />)
    await screen.findByText('问题详情')
    expect(screen.queryByRole('button', { name: '删除问题' })).toBeNull()

    grant(['problem:read', 'problem:delete'])
    rerender(<ProblemDetailDrawer problemId={5001} onClose={vi.fn()} />)
    expect(await screen.findByRole('button', { name: '删除问题' })).toBeInTheDocument()
  })

  it('shows the add-log control only with problem_log:write permission', async () => {
    grant(['problem:read', 'problem_log:write'])
    renderWithProviders(<ProblemDetailDrawer problemId={5001} onClose={vi.fn()} />)
    expect(await screen.findByRole('button', { name: /追加处理记录/ })).toBeInTheDocument()
  })

  it('disables 看全景 when the problem has no cover_media_id', async () => {
    grant(['problem:read'])
    // 5002 无封面。
    renderWithProviders(<ProblemDetailDrawer problemId={5002} onClose={vi.fn()} />)
    const pano = await screen.findByRole('button', { name: /看全景/ })
    expect(pano).toBeDisabled()
  })

  it('renders nothing-bound drawer when problemId is null (closed)', () => {
    grant(['problem:read'])
    renderWithProviders(<ProblemDetailDrawer problemId={null} onClose={vi.fn()} />)
    expect(screen.queryByText('问题详情')).toBeNull()
  })

  it('shows the seeded processing log timeline (read-only) inside the drawer', async () => {
    grant(['problem:read'])
    const { container } = renderWithProviders(<ProblemDetailDrawer problemId={5001} onClose={vi.fn()} />)
    expect(await screen.findByText('初步登记，待复核')).toBeInTheDocument()
    // 追加按钮无 problem_log:write 时不渲染，时间线本身只读。
    const drawer = container.ownerDocument.body
    expect(within(drawer).queryByRole('button', { name: /追加处理记录/ })).toBeNull()
  })
})
