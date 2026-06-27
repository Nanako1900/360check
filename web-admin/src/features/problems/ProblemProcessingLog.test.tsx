import { describe, expect, it } from 'vitest'
import { screen, within } from '@testing-library/react'
import { renderWithProviders } from '@/test/renderWithProviders'
import { ProblemProcessingLog } from './ProblemProcessingLog'

describe('ProblemProcessingLog', () => {
  it('renders the seeded COMMENT log on a read-only / append-only timeline', async () => {
    renderWithProviders(<ProblemProcessingLog problemId={5001} />)
    // 种子 COMMENT 日志的备注。
    expect(await screen.findByText('初步登记，待复核')).toBeInTheDocument()
    expect(screen.getByText('备注')).toBeInTheDocument()
  })

  it('exposes NO edit/delete controls (append-only audit trail)', async () => {
    const { container } = renderWithProviders(<ProblemProcessingLog problemId={5001} />)
    await screen.findByText('初步登记，待复核')
    // 时间线是只读的：不渲染任何编辑/删除按钮。
    const scope = within(container)
    expect(scope.queryByRole('button', { name: /编辑|删除|edit|delete/i })).toBeNull()
    expect(container.querySelectorAll('button').length).toBe(0)
  })

  it('shows the empty state for a problem with no logs', async () => {
    renderWithProviders(<ProblemProcessingLog problemId={5002} />)
    expect(await screen.findByText('暂无处理记录。')).toBeInTheDocument()
  })
})
