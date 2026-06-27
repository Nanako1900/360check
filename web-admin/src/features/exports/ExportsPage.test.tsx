import { describe, expect, it } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/renderWithProviders'
import { ExportsPage } from './ExportsPage'

describe('ExportsPage', () => {
  it('lists the three export surfaces with links to their filter pages', () => {
    renderWithProviders(<ExportsPage />)
    expect(screen.getByText('导出巡查记录 Excel')).toBeInTheDocument()
    expect(screen.getByText('导出问题列表 Excel')).toBeInTheDocument()
    expect(screen.getByText('导出项目统计数据 Excel')).toBeInTheDocument()

    const links = screen.getAllByRole('link')
    const hrefs = links.map((l) => l.getAttribute('href'))
    expect(hrefs).toContain('/inspections')
    expect(hrefs).toContain('/problems')
    expect(hrefs).toContain('/stats')
  })
})
