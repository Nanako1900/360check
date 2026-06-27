import { describe, expect, it } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router-dom'
import { db } from '@/mocks/db'
import { renderWithProviders } from '@/test/renderWithProviders'
import { ProjectListPage } from './ProjectListPage'

function renderList() {
  return renderWithProviders(
    <Routes>
      <Route path="/projects" element={<ProjectListPage />} />
      <Route path="/projects/:id" element={<div>详情占位</div>} />
    </Routes>,
    { route: '/projects' },
  )
}

describe('ProjectListPage', () => {
  it('renders seeded projects with status tags', async () => {
    renderList()
    expect(await screen.findByText('PRJ-2026-001')).toBeInTheDocument()
    expect(screen.getByText('滨江绿道巡查')).toBeInTheDocument()
    // ACTIVE → 进行中 tag
    expect(screen.getAllByText('进行中').length).toBeGreaterThan(0)
  })

  it('navigates to project detail when clicking the code link', async () => {
    const user = userEvent.setup()
    renderList()
    await screen.findByText('PRJ-2026-001')
    await user.click(screen.getByRole('link', { name: 'PRJ-2026-001' }))
    expect(await screen.findByText('详情占位')).toBeInTheDocument()
  })

  it('creates a project through the modal', async () => {
    const user = userEvent.setup()
    renderList()
    await screen.findByText('PRJ-2026-001')

    await user.click(screen.getByRole('button', { name: /新建项目/ }))
    const dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByLabelText('项目编码'), 'PRJ-LIST-001')
    await user.type(within(dialog).getByLabelText('项目名称'), '列表新建项目')
    await user.click(within(dialog).getByRole('button', { name: /确\s*定/ }))

    await waitFor(() => expect(db.projects.some((p) => p.code === 'PRJ-LIST-001')).toBe(true))
  })

  it('shows a friendly inline message when deleting a project with inspections (409)', async () => {
    const user = userEvent.setup()
    renderList()
    await screen.findByText('PRJ-2026-001')

    const rows = screen.getAllByRole('row')
    const targetRow = rows.find((r) => within(r).queryByText('PRJ-2026-001'))
    await user.click(within(targetRow as HTMLElement).getByRole('button', { name: '删除' }))
    const confirm = await screen.findByRole('button', { name: /确\s*定/ })
    await user.click(confirm)

    // 后端 RESTRICT 409 → 友好提示，项目仍在
    expect(await screen.findByText('该项目存在巡查记录，无法删除')).toBeInTheDocument()
    expect(db.projects.some((p) => p.id === 1)).toBe(true)
  })
})
