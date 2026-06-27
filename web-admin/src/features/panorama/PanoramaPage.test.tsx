import { describe, expect, it, vi } from 'vitest'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/renderWithProviders'
import { PanoramaPage } from './PanoramaPage'

// 页面未注入 viewerFactory → PanoramaViewer 会动态 import PSV core；这里 mock 掉避免真实 WebGL。
vi.mock('@photo-sphere-viewer/core', () => ({
  Viewer: class {
    addEventListener(): void {}
    destroy(): void {}
  },
}))

describe('PanoramaPage', () => {
  it('renders the entry form and does not open the modal by default', () => {
    const { queryByRole, getByText } = renderWithProviders(<PanoramaPage />, { route: '/panorama' })
    expect(getByText('全景照片')).toBeInTheDocument()
    // 无 ?media_id → 弹层关闭（无 dialog）。
    expect(queryByRole('dialog')).toBeNull()
  })

  it('opens the panorama modal when clicking 查看全景', async () => {
    const user = userEvent.setup()
    const { getByRole, findByRole } = renderWithProviders(<PanoramaPage />, { route: '/panorama' })
    await user.click(getByRole('button', { name: '查看全景' }))
    const dialog = await findByRole('dialog')
    expect(dialog).toBeInTheDocument()
  })

  it('auto-opens the modal from ?media_id= in the URL', async () => {
    const { findByRole } = renderWithProviders(<PanoramaPage />, { route: '/panorama?media_id=7101' })
    const dialog = await findByRole('dialog')
    expect(dialog).toBeInTheDocument()
  })
})
