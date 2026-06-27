import { describe, expect, it } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/renderWithProviders'
import { DictTag } from './DictTag'

describe('DictTag', () => {
  it('renders an active item label', async () => {
    renderWithProviders(<DictTag code="problem_status" itemId={101} />)
    expect(await screen.findByText('待处理')).toBeInTheDocument()
  })

  it('renders a retired (is_active=false) item with a 已停用 suffix (history tolerance)', async () => {
    renderWithProviders(<DictTag code="problem_status" itemId={199} />)
    expect(await screen.findByText('历史状态')).toBeInTheDocument()
    expect(screen.getByText('（已停用）')).toBeInTheDocument()
  })

  it('renders a dash for a null itemId', () => {
    renderWithProviders(<DictTag code="problem_status" itemId={null} />)
    expect(screen.getByText('—')).toBeInTheDocument()
  })
})
