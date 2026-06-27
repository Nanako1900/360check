import { describe, expect, it, vi } from 'vitest'
import { act, render, waitFor } from '@testing-library/react'
import { renderWithProviders } from '@/test/renderWithProviders'
import { PanoramaViewer } from './PanoramaViewer'
import type { PanoViewerConfig, PanoViewerLike } from './PanoramaViewer'

// 永不加载真实 WebGL：mock @photo-sphere-viewer/core，记录构造参数与事件监听。
const ctorCalls: PanoViewerConfig[] = []
const destroyCalls = { count: 0 }
const listeners: Record<string, (e: unknown) => void> = {}

class FakeViewer {
  constructor(config: PanoViewerConfig) {
    ctorCalls.push(config)
  }
  addEventListener(type: string, cb: (e: unknown) => void): void {
    listeners[type] = cb
  }
  destroy(): void {
    destroyCalls.count += 1
  }
}

vi.mock('@photo-sphere-viewer/core', () => ({
  Viewer: FakeViewer,
}))

function resetSpies(): void {
  ctorCalls.length = 0
  destroyCalls.count = 0
  for (const k of Object.keys(listeners)) delete listeners[k]
}

describe('PanoramaViewer (mocked PSV core via dynamic import)', () => {
  it('constructs the Viewer with the given panorama URL and tears it down on unmount', async () => {
    resetSpies()
    const url = 'https://cdn.example.com/web/7101.jpg?sign=abc'
    const { unmount } = renderWithProviders(<PanoramaViewer panorama={url} />)

    await waitFor(() => expect(ctorCalls).toHaveLength(1))
    expect(ctorCalls[0].panorama).toBe(url)
    expect(ctorCalls[0].container).toBeInstanceOf(HTMLElement)

    unmount()
    expect(destroyCalls.count).toBe(1)
  })

  it('rebuilds the Viewer (destroy old + construct new) when the panorama URL changes', async () => {
    resetSpies()
    const { rerender } = renderWithProviders(
      <PanoramaViewer panorama="https://cdn.example.com/a.jpg" />,
    )
    await waitFor(() => expect(ctorCalls).toHaveLength(1))

    rerender(<PanoramaViewer panorama="https://cdn.example.com/b.jpg" />)
    await waitFor(() => expect(ctorCalls).toHaveLength(2))
    expect(ctorCalls[1].panorama).toBe('https://cdn.example.com/b.jpg')
    // 旧实例被销毁。
    expect(destroyCalls.count).toBeGreaterThanOrEqual(1)
  })

  it('calls onExpired and shows the error state when PSV emits panorama-error', async () => {
    resetSpies()
    const onExpired = vi.fn()
    const { getByRole } = renderWithProviders(
      <PanoramaViewer panorama="https://cdn.example.com/x.jpg" onExpired={onExpired} />,
    )
    await waitFor(() => expect(listeners['panorama-error']).toBeTypeOf('function'))

    act(() => listeners['panorama-error']({}))
    expect(onExpired).toHaveBeenCalledTimes(1)
    await waitFor(() => expect(getByRole('alert')).toBeInTheDocument())
  })
})

describe('PanoramaViewer (injected factory, no module load)', () => {
  it('uses the injected viewerFactory and reaches ready on the ready event', async () => {
    let captured: PanoViewerConfig | null = null
    const handlers: Record<string, (e: unknown) => void> = {}
    const fakeViewer: PanoViewerLike = {
      addEventListener: (type, cb) => {
        handlers[type] = cb
      },
      destroy: vi.fn(),
    }
    const factory = vi.fn((config: PanoViewerConfig): PanoViewerLike => {
      captured = config
      return fakeViewer
    })

    const { queryByRole } = render(
      <PanoramaViewer panorama="https://cdn.example.com/web.jpg" viewerFactory={factory} />,
    )
    await waitFor(() => expect(factory).toHaveBeenCalledTimes(1))
    expect(captured).not.toBeNull()
    expect((captured as unknown as PanoViewerConfig).panorama).toBe(
      'https://cdn.example.com/web.jpg',
    )

    act(() => handlers['ready']({}))
    await waitFor(() => expect(queryByRole('alert')).toBeNull())
  })
})
