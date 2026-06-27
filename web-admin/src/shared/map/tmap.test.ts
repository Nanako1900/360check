import { afterEach, describe, expect, it } from 'vitest'
import { __resetSdkSingleton, loadTencentSdk } from './tmap'
import { installFakeTMap, uninstallFakeTMap } from '@/test/fakeTMap'

afterEach(() => {
  uninstallFakeTMap()
  __resetSdkSingleton()
  document.getElementById('tencent-map-gl-sdk')?.remove()
})

describe('loadTencentSdk', () => {
  it('resolves immediately when window.TMap already exists', async () => {
    const handles = installFakeTMap()
    await expect(loadTencentSdk('k')).resolves.toBe(handles.TMap)
  })

  it('rejects when key is missing and no TMap present', async () => {
    delete window.TMap
    __resetSdkSingleton()
    await expect(loadTencentSdk('')).rejects.toThrow(/key/)
  })

  it('injects a <script> tag and resolves on load once window.TMap appears', async () => {
    delete window.TMap
    __resetSdkSingleton()
    const promise = loadTencentSdk('my-key')
    const script = document.getElementById('tencent-map-gl-sdk') as HTMLScriptElement | null
    expect(script).not.toBeNull()
    expect(script?.src).toContain('my-key')
    // 模拟 SDK 注入全局后触发 load。
    const handles = installFakeTMap()
    script?.dispatchEvent(new Event('load'))
    await expect(promise).resolves.toBe(handles.TMap)
  })

  it('rejects on script error', async () => {
    delete window.TMap
    __resetSdkSingleton()
    const promise = loadTencentSdk('my-key')
    const script = document.getElementById('tencent-map-gl-sdk') as HTMLScriptElement | null
    script?.dispatchEvent(new Event('error'))
    await expect(promise).rejects.toThrow(/失败/)
  })

  it('rejects when script loads but window.TMap is still absent', async () => {
    delete window.TMap
    __resetSdkSingleton()
    const promise = loadTencentSdk('my-key')
    const script = document.getElementById('tencent-map-gl-sdk') as HTMLScriptElement | null
    script?.dispatchEvent(new Event('load'))
    await expect(promise).rejects.toThrow(/window\.TMap/)
  })
})
