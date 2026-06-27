import { describe, expect, it } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/renderWithProviders'
import { useAuthStore } from '@/shared/auth/authStore'
import { LoginPage } from './LoginPage'

describe('LoginPage', () => {
  it('shows required-field errors on empty submit', async () => {
    const user = userEvent.setup()
    renderWithProviders(<LoginPage />, { route: '/login' })
    await user.click(screen.getByRole('button', { name: /登/ }))
    expect(await screen.findByText('请输入用户名')).toBeInTheDocument()
    expect(await screen.findByText('请输入密码')).toBeInTheDocument()
  })

  it('logs in and populates token + permission codes from /auth/me', async () => {
    const user = userEvent.setup()
    renderWithProviders(<LoginPage />, { route: '/login' })
    await user.type(screen.getByPlaceholderText('请输入用户名'), 'admin')
    await user.type(screen.getByPlaceholderText('请输入密码'), 'admin12345')
    await user.click(screen.getByRole('button', { name: /登/ }))

    await waitFor(() => expect(useAuthStore.getState().accessToken).toBeTruthy())
    expect(useAuthStore.getState().permissionCodes.length).toBeGreaterThan(0)
  })

  it('surfaces a friendly error on wrong credentials', async () => {
    const user = userEvent.setup()
    renderWithProviders(<LoginPage />, { route: '/login' })
    await user.type(screen.getByPlaceholderText('请输入用户名'), 'admin')
    await user.type(screen.getByPlaceholderText('请输入密码'), 'wrong-pass')
    await user.click(screen.getByRole('button', { name: /登/ }))

    expect(await screen.findByText('用户名或密码错误')).toBeInTheDocument()
    expect(useAuthStore.getState().accessToken).toBeNull()
  })
})
