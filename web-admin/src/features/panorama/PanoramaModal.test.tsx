import { describe, expect, it, vi } from 'vitest'
import { act, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { server } from '@/mocks/server'
import { renderWithProviders } from '@/test/renderWithProviders'
import type { PanoViewerConfig, PanoViewerLike } from '@/shared/psv/PanoramaViewer'
import { PanoramaModal } from './PanoramaModal'
import type { PanoramaContext } from './PanoramaModal'

const BASE = '*/api/v1'

/** 工厂：记录每次构造的 config 与事件监听器，便于断言 URL / 触发 panorama-error。 */
function makeFactory() {
  const configs: PanoViewerConfig[] = []
  const listeners: Record<string, (e: unknown) => void> = {}
  const factory = vi.fn((config: PanoViewerConfig): PanoViewerLike => {
    configs.push(config)
    return {
      addEventListener: (type, cb) => {
        listeners[type] = cb
      },
      destroy: vi.fn(),
    }
  })
  return { factory, configs, listeners }
}

const CONTEXT: PanoramaContext = {
  projectName: '滨江绿道巡查',
  inspectionStartedAt: '2026-06-20T01:00:00Z',
  inspectorName: '李巡查',
  problemTypeItemId: 201,
  problemStatusItemId: 101,
  problemTitle: '步道路面横向裂缝',
}

describe('PanoramaModal', () => {
  it('renders associated context labels and the panorama for a CONFIRMED web media', async () => {
    const { factory, configs } = makeFactory()
    const { findByText } = renderWithProviders(
      <PanoramaModal open mediaId={7101} context={CONTEXT} onClose={vi.fn()} viewerFactory={factory} />,
    )

    // 关联上下文渲染（项目 / 巡查 / 问题标签区）。
    expect(await findByText('所属项目')).toBeInTheDocument()
    expect(await findByText('滨江绿道巡查')).toBeInTheDocument()
    expect(await findByText('巡查记录')).toBeInTheDocument()
    expect(await findByText('李巡查')).toBeInTheDocument()
    expect(await findByText('关联问题')).toBeInTheDocument()
    expect(await findByText('步道路面横向裂缝')).toBeInTheDocument()

    // 用 CONFIRMED web 媒体的签名 URL 构造了 PSV Viewer。
    await waitFor(() => expect(factory).toHaveBeenCalledTimes(1))
    expect(configs[0].panorama).toContain('https://cdn.example.com/web/7101.jpg')
  })

  it('shows the unconfirmed empty state and never constructs a viewer for unconfirmed media', async () => {
    const { factory } = makeFactory()
    const { findByText } = renderWithProviders(
      <PanoramaModal open mediaId={7103} onClose={vi.fn()} viewerFactory={factory} />,
    )
    expect(await findByText('照片处理中或未确认，暂不可查看。')).toBeInTheDocument()
    expect(factory).not.toHaveBeenCalled()
  })

  it('refetches a fresh signed_url when the viewer reports expiry (panorama-error)', async () => {
    let mediaHits = 0
    server.use(
      http.get(`${BASE}/media/:id`, () => {
        mediaHits += 1
        return HttpResponse.json({
          success: true,
          error: null,
          meta: null,
          data: {
            id: 7101,
            client_uuid: 'cccccccc-0000-0000-0000-000000007101',
            owner_type: 'problem',
            owner_id: 5001,
            tier: 'web',
            cos_bucket: 'b',
            cos_key: 'k',
            cos_region: 'ap-shanghai',
            capture_state: 'CONFIRMED',
            verified_at: '2026-06-26T00:00:00Z',
            signed_url: `https://cdn.example.com/web/7101.jpg?sign=v${mediaHits}`,
            created_at: '2026-06-26T00:00:00Z',
            updated_at: '2026-06-26T00:00:00Z',
          },
        })
      }),
    )

    const { factory, listeners } = makeFactory()
    renderWithProviders(
      <PanoramaModal open mediaId={7101} onClose={vi.fn()} viewerFactory={factory} />,
    )

    await waitFor(() => expect(factory).toHaveBeenCalledTimes(1))
    expect(mediaHits).toBe(1)

    // 模拟签名过期：PSV 触发 panorama-error → onExpired → refetch。
    act(() => listeners['panorama-error']({}))
    await waitFor(() => expect(mediaHits).toBe(2))
  })

  it('does not fetch media or render the viewer while closed', async () => {
    const { factory } = makeFactory()
    let mediaHits = 0
    server.use(
      http.get(`${BASE}/media/:id`, () => {
        mediaHits += 1
        return HttpResponse.json({ success: true, data: null, error: null, meta: null })
      }),
    )
    renderWithProviders(
      <PanoramaModal open={false} mediaId={7101} onClose={vi.fn()} viewerFactory={factory} />,
    )
    await new Promise((r) => setTimeout(r, 20))
    expect(mediaHits).toBe(0)
    expect(factory).not.toHaveBeenCalled()
  })
})
