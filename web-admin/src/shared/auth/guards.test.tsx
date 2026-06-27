import { describe, expect, it } from 'vitest'
import { screen } from '@testing-library/react'
import { Route, Routes } from 'react-router-dom'
import { renderWithProviders } from '@/test/renderWithProviders'
import { RequireAuth, RequirePermission } from './guards'
import { useAuthStore } from './authStore'
import type { User } from '@/shared/api/types'

const user: User = {
  id: 1,
  username: 'a',
  display_name: 'A',
  is_active: true,
  created_at: '2026-06-26T00:00:00Z',
  updated_at: '2026-06-26T00:00:00Z',
}

describe('guards', () => {
  it('RequireAuth redirects to /login without a token', () => {
    renderWithProviders(
      <Routes>
        <Route path="/login" element={<div>登录页</div>} />
        <Route
          path="/secret"
          element={
            <RequireAuth>
              <div>机密</div>
            </RequireAuth>
          }
        />
      </Routes>,
      { route: '/secret' },
    )
    expect(screen.getByText('登录页')).toBeInTheDocument()
    expect(screen.queryByText('机密')).not.toBeInTheDocument()
  })

  it('RequirePermission renders Forbidden without the permission', () => {
    renderWithProviders(
      <RequirePermission code="x:read">
        <div>内容</div>
      </RequirePermission>,
    )
    expect(screen.getByText('403')).toBeInTheDocument()
  })

  it('RequirePermission renders children when permission is present', () => {
    useAuthStore.getState().setMe({ user, roles: [], permissions: ['x:read'] })
    renderWithProviders(
      <RequirePermission code="x:read">
        <div>内容</div>
      </RequirePermission>,
    )
    expect(screen.getByText('内容')).toBeInTheDocument()
  })
})
