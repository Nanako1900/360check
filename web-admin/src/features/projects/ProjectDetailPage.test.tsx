import { describe, expect, it } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router-dom'
import { db } from '@/mocks/db'
import { renderWithProviders } from '@/test/renderWithProviders'
import { ProjectDetailPage } from './ProjectDetailPage'

function renderDetail(id: number) {
  return renderWithProviders(
    <Routes>
      <Route path="/projects/:id" element={<ProjectDetailPage />} />
      <Route path="/inspections" element={<div>巡查列表占位</div>} />
    </Routes>,
    { route: `/projects/${id}` },
  )
}

describe('ProjectDetailPage', () => {
  it('renders project detail with custom_fields resolved via dict labels', async () => {
    renderDetail(1)
    expect(await screen.findByText('滨江绿道巡查')).toBeInTheDocument()
    // custom_fields region/budget labelled by project_field dict items
    expect(await screen.findByText('所属区域')).toBeInTheDocument()
    expect(screen.getByText('滨江区')).toBeInTheDocument()
    expect(screen.getByText('预算（万元）')).toBeInTheDocument()
  })

  it('renders related tasks and related inspections sections', async () => {
    renderDetail(1)
    await screen.findByText('滨江绿道巡查')
    expect(await screen.findByText('关联任务')).toBeInTheDocument()
    expect(screen.getByText('巡查记录')).toBeInTheDocument()
    // seeded task title under this project
    expect(await screen.findByText('一号段步道巡查')).toBeInTheDocument()
  })

  it('creates a task assigned to an inspector through the task modal', async () => {
    const user = userEvent.setup()
    renderDetail(1)
    await screen.findByText('一号段步道巡查')

    await user.click(screen.getByRole('button', { name: /新建任务/ }))
    const dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByLabelText('任务标题'), '临时巡查任务')
    await user.click(within(dialog).getByRole('button', { name: /确\s*定/ }))

    await waitFor(() =>
      expect(db.tasks.some((tk) => tk.title === '临时巡查任务' && tk.project_id === 1)).toBe(true),
    )
  })

  it('shows not-found state for a missing project', async () => {
    renderDetail(9999)
    expect(await screen.findByText('项目不存在或已被删除。')).toBeInTheDocument()
  })
})
