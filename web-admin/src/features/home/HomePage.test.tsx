import { describe, expect, it } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/renderWithProviders'
import { useAuthStore } from '@/shared/auth/authStore'
import { USERS } from '@/mocks/db'
import { HomePage } from './HomePage'

describe('HomePage', () => {
  it('greets the current user and shows the permission count', () => {
    useAuthStore.getState().setMe({ user: USERS[0], roles: [], permissions: ['a', 'b'] })
    renderWithProviders(<HomePage />)
    expect(screen.getAllByText('工作台').length).toBeGreaterThan(0)
    expect(screen.getByText('2')).toBeInTheDocument()
  })
})
