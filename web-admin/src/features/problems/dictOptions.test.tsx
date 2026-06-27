import { describe, expect, it } from 'vitest'
import type { ReactNode } from 'react'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { clearDictCache } from '@/shared/dict/dictCache'
import { useActiveDictOptions } from './dictOptions'

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
  return { Wrapper }
}

describe('useActiveDictOptions', () => {
  it('returns ONLY active items (retired 199 excluded) for problem_status', async () => {
    clearDictCache()
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useActiveDictOptions('problem_status'), { wrapper: Wrapper })
    await waitFor(() => expect(result.current.length).toBeGreaterThan(0))
    const values = result.current.map((o) => o.value)
    // 退役项 199 不进入筛选/编辑下拉。
    expect(values).not.toContain(199)
    expect(values).toContain(101)
    expect(values).toContain(102)
  })
})
