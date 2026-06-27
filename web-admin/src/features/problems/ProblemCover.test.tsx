import { describe, expect, it } from 'vitest'
import { waitFor } from '@testing-library/react'
import { renderWithProviders } from '@/test/renderWithProviders'
import { ProblemCover } from './ProblemCover'

describe('ProblemCover', () => {
  it('renders a placeholder when there is no cover_media_id', () => {
    const { container } = renderWithProviders(<ProblemCover mediaId={null} />)
    // 占位：无 img 标签。
    expect(container.querySelector('img')).toBeNull()
  })

  it('renders the thumb image once the signed_url is fetched for a confirmed media', async () => {
    // 媒体 7102 是 thumb（CONFIRMED，含签名 URL）。
    const { container } = renderWithProviders(<ProblemCover mediaId={7102} />)
    await waitFor(() => {
      const img = container.querySelector('img')
      expect(img).not.toBeNull()
      expect(img?.src).toContain('https://cdn.example.com/thumb/7102.jpg')
    })
  })

  it('falls back to a placeholder for media without a signed_url (unconfirmed)', async () => {
    // 媒体 7103 未确认（signed_url=null）。
    const { container } = renderWithProviders(<ProblemCover mediaId={7103} />)
    await new Promise((r) => setTimeout(r, 20))
    expect(container.querySelector('img')).toBeNull()
  })
})
