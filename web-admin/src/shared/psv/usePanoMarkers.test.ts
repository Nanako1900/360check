import { describe, expect, it, vi } from 'vitest'
import { renderHook } from '@testing-library/react'
import { usePanoMarkers } from './usePanoMarkers'
import type { PanoMarker, PanoMarkersPluginLike } from './usePanoMarkers'

function fakePlugin(): PanoMarkersPluginLike & {
  added: Array<{ id: string; html?: string }>
  cleared: number
} {
  const added: Array<{ id: string; html?: string }> = []
  let cleared = 0
  return {
    added,
    get cleared() {
      return cleared
    },
    clearMarkers: () => {
      cleared += 1
    },
    addMarker: (config) => {
      added.push({ id: config.id, html: config.html })
    },
  }
}

describe('usePanoMarkers', () => {
  it('clears then adds each marker on the plugin', () => {
    const plugin = fakePlugin()
    const markers: PanoMarker[] = [
      { id: 'm1', yaw: 0, pitch: 0, tooltip: '问题点' },
      { id: 'm2', yaw: '30deg', pitch: '10deg', html: '<b>注记</b>' },
    ]
    renderHook(() => usePanoMarkers(plugin, markers))
    expect(plugin.cleared).toBe(1)
    expect(plugin.added.map((m) => m.id)).toEqual(['m1', 'm2'])
    // 默认圆点用于未提供 html 的标记。
    expect(plugin.added[0].html).toContain('border-radius')
    expect(plugin.added[1].html).toBe('<b>注记</b>')
  })

  it('is a no-op when the plugin is null', () => {
    const markers: PanoMarker[] = [{ id: 'm1', yaw: 0, pitch: 0 }]
    expect(() => renderHook(() => usePanoMarkers(null, markers))).not.toThrow()
  })

  it('re-syncs when the markers list changes', () => {
    const plugin = fakePlugin()
    const addMarker = vi.spyOn(plugin, 'addMarker')
    const { rerender } = renderHook(({ ms }: { ms: PanoMarker[] }) => usePanoMarkers(plugin, ms), {
      initialProps: { ms: [{ id: 'm1', yaw: 0, pitch: 0 }] as PanoMarker[] },
    })
    rerender({ ms: [{ id: 'a', yaw: 1, pitch: 1 }, { id: 'b', yaw: 2, pitch: 2 }] })
    expect(addMarker).toHaveBeenCalledTimes(3) // 1 + 2
  })
})
