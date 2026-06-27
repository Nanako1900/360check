import { afterEach, describe, expect, it, vi } from 'vitest'
import { render } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ProblemFeatureProperties } from '@/shared/api/types'
import { installFakeTMap, uninstallFakeTMap } from '@/test/fakeTMap'
import { buildInfoWindowContent, escapeHtml, safeColor } from './infoWindowContent'
import type { InfoWindowLabels } from './infoWindowContent'
import { MapInfoWindow } from './MapInfoWindow'
import type { InfoWindowTarget } from './MapInfoWindow'
import type { TMapMap } from './tmap'

afterEach(() => {
  uninstallFakeTMap()
})

const LABELS: InfoWindowLabels = {
  type: '类型',
  status: '状态',
  viewPanorama: '看全景',
  viewDetail: '看详情',
  noThumb: '暂无缩略图',
  untitled: '未命名问题',
}

function fakeMap(): TMapMap {
  return {
    setCenter: vi.fn(),
    setZoom: vi.fn(),
    fitBounds: vi.fn(),
    destroy: vi.fn(),
    on: vi.fn(),
    off: vi.fn(),
  }
}

describe('infoWindowContent (pure, security)', () => {
  it('escapes HTML special characters to prevent XSS', () => {
    expect(escapeHtml('<script>alert(1)</script>')).toBe(
      '&lt;script&gt;alert(1)&lt;/script&gt;',
    )
    expect(escapeHtml(`a"b'c&d`)).toBe('a&quot;b&#39;c&amp;d')
  })

  it('rejects non-hex colors and falls back to a neutral color', () => {
    expect(safeColor('#1677ff')).toBe('#1677ff')
    expect(safeColor('javascript:alert(1)')).toBe('#64748b')
    expect(safeColor(null)).toBe('#64748b')
  })

  it('builds content with escaped title and labels', () => {
    const props: ProblemFeatureProperties = {
      id: 1,
      title: '<b>裂缝</b>',
      type_label: '裂缝',
      status_label: '待处理',
      status_color: '#8c8c8c',
      cover_media_id: null,
    }
    const html = buildInfoWindowContent(props, LABELS)
    expect(html).toContain('&lt;b&gt;裂缝&lt;/b&gt;')
    expect(html).not.toContain('<b>裂缝</b>')
    expect(html).toContain('暂无缩略图')
    expect(html).toContain('看全景')
    expect(html).toContain('看详情')
  })

  it('renders the cover placeholder when cover_media_id is present', () => {
    const props: ProblemFeatureProperties = { id: 2, cover_media_id: 7001 }
    const html = buildInfoWindowContent(props, LABELS)
    expect(html).toContain('#7001')
    expect(html).toContain('未命名问题')
  })
})

describe('MapInfoWindow component', () => {
  it('opens the InfoWindow with content when a target is set', () => {
    const handles = installFakeTMap()
    const map = fakeMap()
    const target: InfoWindowTarget = {
      position: [116.404, 39.915],
      properties: { id: 1, title: '裂缝', status_color: '#8c8c8c' },
    }
    render(
      <MapInfoWindow
        map={map}
        TMap={handles.TMap}
        target={target}
        labels={LABELS}
        onViewPanorama={vi.fn()}
        onViewDetail={vi.fn()}
      />,
    )
    expect(handles.infoInstance?.open).toHaveBeenCalled()
    expect(handles.infoInstance?.lastContent).toContain('裂缝')
  })

  it('fires onViewDetail / onViewPanorama when delegated buttons are clicked', async () => {
    const user = userEvent.setup()
    const handles = installFakeTMap()
    const map = fakeMap()
    const onViewDetail = vi.fn()
    const onViewPanorama = vi.fn()
    const target: InfoWindowTarget = {
      position: [116.404, 39.915],
      properties: { id: 42, title: '裂缝' },
    }
    render(
      <MapInfoWindow
        map={map}
        TMap={handles.TMap}
        target={target}
        labels={LABELS}
        onViewPanorama={onViewPanorama}
        onViewDetail={onViewDetail}
      />,
    )
    // InfoWindow 内容是注入 HTML：手动注入到 DOM 模拟腾讯地图气泡，再走 document 委托。
    const container = document.createElement('div')
    container.innerHTML = handles.infoInstance?.lastContent ?? ''
    document.body.appendChild(container)

    const detailBtn = container.querySelector('[data-map-info-action="detail"]') as HTMLElement
    const panoBtn = container.querySelector('[data-map-info-action="panorama"]') as HTMLElement
    await user.click(detailBtn)
    await user.click(panoBtn)
    expect(onViewDetail).toHaveBeenCalledTimes(1)
    expect(onViewDetail.mock.calls[0][0].id).toBe(42)
    expect(onViewPanorama).toHaveBeenCalledTimes(1)
    document.body.removeChild(container)
  })

  it('closes the InfoWindow when target becomes null and destroys on unmount', () => {
    const handles = installFakeTMap()
    const map = fakeMap()
    const { rerender, unmount } = render(
      <MapInfoWindow
        map={map}
        TMap={handles.TMap}
        target={{ position: [116.404, 39.915], properties: { id: 1 } }}
        labels={LABELS}
        onViewPanorama={vi.fn()}
        onViewDetail={vi.fn()}
      />,
    )
    const info = handles.infoInstance
    rerender(
      <MapInfoWindow
        map={map}
        TMap={handles.TMap}
        target={null}
        labels={LABELS}
        onViewPanorama={vi.fn()}
        onViewDetail={vi.fn()}
      />,
    )
    expect(info?.close).toHaveBeenCalled()
    unmount()
    expect(info?.destroy).toHaveBeenCalled()
  })
})
