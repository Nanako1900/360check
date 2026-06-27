import '@testing-library/jest-dom/vitest'
import { afterAll, afterEach, beforeAll } from 'vitest'
import { server } from '@/mocks/server'
import { resetDb } from '@/mocks/db'
import { useAuthStore } from '@/shared/auth/authStore'
import { clearDictCache } from '@/shared/dict/dictCache'

// —— jsdom 下 antd 依赖的浏览器 API polyfill ——
function installBrowserPolyfills(): void {
  if (typeof window.matchMedia !== 'function') {
    window.matchMedia = ((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    })) as unknown as typeof window.matchMedia
  }
  if (typeof globalThis.ResizeObserver === 'undefined') {
    class ResizeObserverStub {
      observe(): void {}
      unobserve(): void {}
      disconnect(): void {}
    }
    globalThis.ResizeObserver = ResizeObserverStub as unknown as typeof ResizeObserver
  }
}

beforeAll(() => {
  installBrowserPolyfills()
  server.listen({ onUnhandledRequest: 'error' })
})

afterEach(() => {
  server.resetHandlers()
  resetDb()
  clearDictCache()
  localStorage.clear()
  sessionStorage.clear()
  useAuthStore.getState().reset()
})

afterAll(() => {
  server.close()
})
