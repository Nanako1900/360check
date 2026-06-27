import { afterEach, describe, expect, it } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { installFakeTMap, uninstallFakeTMap } from '@/test/fakeTMap'
import { loadTencentSdk } from './tmap'
import { useTencentMap } from './useTencentMap'

afterEach(() => {
  uninstallFakeTMap()
})

describe('useTencentMap', () => {
  it('reaches ready status and constructs a Map when window.TMap is injected', async () => {
    const handles = installFakeTMap()
    const { result } = renderHook(() => useTencentMap({ apiKey: 'test-key' }))

    // 容器 ref 必须在初始化前挂上节点（hook 读取 containerRef.current）。
    const container = document.createElement('div')
    ;(result.current.containerRef as { current: HTMLDivElement | null }).current = container

    await waitFor(() => expect(result.current.status).toBe('ready'))
    expect(result.current.map).not.toBeNull()
    expect(result.current.TMap).toBe(handles.TMap)
    expect(handles.Map).toHaveBeenCalledTimes(1)
  })

  it('destroys the map instance on unmount (no leak)', async () => {
    const handles = installFakeTMap()
    const { result, unmount } = renderHook(() => useTencentMap({ apiKey: 'test-key' }))
    const container = document.createElement('div')
    ;(result.current.containerRef as { current: HTMLDivElement | null }).current = container

    await waitFor(() => expect(result.current.status).toBe('ready'))
    const instance = handles.mapInstance
    expect(instance).not.toBeNull()

    unmount()
    expect(instance?.destroy).toHaveBeenCalledTimes(1)
  })

  it('loadTencentSdk resolves immediately when window.TMap already present', async () => {
    const handles = installFakeTMap()
    await expect(loadTencentSdk('test-key')).resolves.toBe(handles.TMap)
  })

  it('reports error status when the container is missing', async () => {
    installFakeTMap()
    const { result } = renderHook(() => useTencentMap({ apiKey: 'test-key' }))
    // 不挂 container.current → 初始化应进入 error 分支。
    await waitFor(() => expect(result.current.status).toBe('error'))
    expect(result.current.error).toBeTruthy()
  })
})
