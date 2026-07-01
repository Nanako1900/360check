import { describe, expect, it, vi } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/renderWithProviders'
import type { Project } from '@/shared/api/types'
import { MapSelectionCard } from './MapSelectionCard'

function mkProject(over: Partial<Project> = {}): Project {
  return {
    id: 7,
    code: 'BJ-01',
    name: '滨江绿道',
    status: 'ACTIVE',
    ...over,
  } as unknown as Project
}

describe('MapSelectionCard', () => {
  it('shows project name, problem count and the three entry buttons', () => {
    renderWithProviders(<MapSelectionCard project={mkProject()} problemCount={3} onClose={vi.fn()} />)
    expect(screen.getByText('滨江绿道')).toBeInTheDocument()
    expect(screen.getByText(/3 个问题/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '项目详情' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '巡查记录' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '问题列表' })).toBeInTheDocument()
  })

  it('omits the problem count when not loaded', () => {
    renderWithProviders(
      <MapSelectionCard project={mkProject()} problemCount={null} onClose={vi.fn()} />,
    )
    expect(screen.queryByText(/个问题/)).not.toBeInTheDocument()
  })

  it('calls onClose when the close button is clicked', async () => {
    const onClose = vi.fn()
    renderWithProviders(<MapSelectionCard project={mkProject()} problemCount={0} onClose={onClose} />)
    await userEvent.click(screen.getByRole('button', { name: '取消' }))
    expect(onClose).toHaveBeenCalledTimes(1)
  })
})
