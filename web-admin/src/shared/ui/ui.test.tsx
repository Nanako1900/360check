import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/renderWithProviders'
import { Forbidden } from './Forbidden'
import { NotFound } from './NotFound'
import { EmptyState } from './EmptyState'
import { ErrorBoundary } from './ErrorBoundary'

function Boom(): never {
  throw new Error('boom')
}

describe('ui primitives', () => {
  it('Forbidden shows a 403 result', () => {
    renderWithProviders(<Forbidden />)
    expect(screen.getByText('403')).toBeInTheDocument()
  })

  it('NotFound shows a 404 result', () => {
    renderWithProviders(<NotFound />)
    expect(screen.getByText('404')).toBeInTheDocument()
  })

  it('EmptyState renders its description', () => {
    renderWithProviders(<EmptyState description="空空如也" />)
    expect(screen.getByText('空空如也')).toBeInTheDocument()
  })

  it('ErrorBoundary renders a fallback when a child throws', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    render(
      <ErrorBoundary>
        <Boom />
      </ErrorBoundary>,
    )
    expect(screen.getByText('页面出错了')).toBeInTheDocument()
    spy.mockRestore()
  })
})
