import { describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/renderWithProviders'
import { ChangePasswordModal } from './ChangePasswordModal'

describe('ChangePasswordModal', () => {
  it('rejects a weak new password', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ChangePasswordModal open onClose={() => {}} />)
    await user.type(screen.getByLabelText('当前密码'), 'admin12345')
    await user.type(screen.getByLabelText('新密码'), 'weak')
    await user.click(screen.getByRole('button', { name: /确\s*定/ }))
    expect(await screen.findByText('密码至少 8 位，需包含字母与数字')).toBeInTheDocument()
  })

  it('rejects a mismatched confirmation', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ChangePasswordModal open onClose={() => {}} />)
    await user.type(screen.getByLabelText('当前密码'), 'admin12345')
    await user.type(screen.getByLabelText('新密码'), 'newpass12')
    await user.type(screen.getByLabelText('确认新密码'), 'different12')
    await user.click(screen.getByRole('button', { name: /确\s*定/ }))
    expect(await screen.findByText('两次输入的新密码不一致')).toBeInTheDocument()
  })

  it('submits successfully and invokes onSuccess + onClose', async () => {
    const onSuccess = vi.fn()
    const onClose = vi.fn()
    const user = userEvent.setup()
    renderWithProviders(<ChangePasswordModal open onClose={onClose} onSuccess={onSuccess} />)
    await user.type(screen.getByLabelText('当前密码'), 'admin12345')
    await user.type(screen.getByLabelText('新密码'), 'newpass12')
    await user.type(screen.getByLabelText('确认新密码'), 'newpass12')
    await user.click(screen.getByRole('button', { name: /确\s*定/ }))
    await waitFor(() => expect(onSuccess).toHaveBeenCalled())
    expect(onClose).toHaveBeenCalled()
  })
})
