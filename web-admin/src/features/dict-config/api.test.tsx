import { beforeEach, describe, expect, it } from 'vitest'
import type { ReactNode } from 'react'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { clearDictCache, getCachedDict, setCachedDict } from '@/shared/dict/dictCache'
import {
  useConfig,
  useConfigHistory,
  useCreateDictType,
  useDictTypes,
  useUpdateConfig,
  useUpdateDictItem,
} from './api'

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { retry: false } },
  })
  function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
  return { qc, Wrapper }
}

beforeEach(() => {
  clearDictCache()
})

describe('dict-config api hooks', () => {
  it('useDictTypes lists seeded types and filters by scope', async () => {
    const { Wrapper } = makeWrapper()
    const all = renderHook(() => useDictTypes(), { wrapper: Wrapper })
    await waitFor(() => expect(all.result.current.isSuccess).toBe(true))
    expect(all.result.current.data?.items.length).toBeGreaterThanOrEqual(4)

    const captureOnly = renderHook(() => useDictTypes('capture_preset'), { wrapper: Wrapper })
    await waitFor(() => expect(captureOnly.result.current.isSuccess).toBe(true))
    expect(captureOnly.result.current.data?.items.every((t) => t.scope === 'capture_preset')).toBe(
      true,
    )
  })

  it('useDictTypes filters by is_active=false (retired)', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useDictTypes(undefined, false), { wrapper: Wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.items.every((t) => t.is_active === false)).toBe(true)
  })

  it('useCreateDictType creates a new type and returns it', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useCreateDictType(), { wrapper: Wrapper })
    const created = await result.current.mutateAsync({
      code: 'new_scope',
      name: '新建',
      scope: 'misc',
    })
    expect(created.code).toBe('new_scope')
    expect(created.id).toBeGreaterThan(0)
  })

  it('useUpdateDictItem retiring an item invalidates the dict cache (DictTag refresh)', async () => {
    const { Wrapper } = makeWrapper()
    // 预置缓存，模拟 DictTag 已拉取过 problem_status
    setCachedDict('problem_status', {
      items: [
        {
          id: 101,
          dict_type_id: 10,
          code: 'OPEN',
          label: '待处理',
          sort_order: 0,
          is_active: true,
        },
      ],
      version: 1,
      contentHash: 'hash-ps-1',
    })
    expect(getCachedDict('problem_status')).toBeDefined()

    const { result } = renderHook(() => useUpdateDictItem(), { wrapper: Wrapper })
    await result.current.mutateAsync({
      id: 101,
      code: 'problem_status',
      body: { is_active: false },
    })
    // onSuccess → clearDictCache() 清空内存缓存
    expect(getCachedDict('problem_status')).toBeUndefined()
  })

  it('useConfig reads a config block and useUpdateConfig bumps version', async () => {
    const { Wrapper } = makeWrapper()
    const read = renderHook(() => useConfig('export.image'), { wrapper: Wrapper })
    await waitFor(() => expect(read.result.current.isSuccess).toBe(true))
    const before = read.result.current.data
    expect(before?.config_key).toBe('export.image')

    const update = renderHook(() => useUpdateConfig('export.image'), { wrapper: Wrapper })
    const next = await update.result.current.mutateAsync({ value: { width: 2048, quality: 60 } })
    expect(next.version).toBe((before?.version ?? 0) + 1)
    expect(next.value).toEqual({ width: 2048, quality: 60 })
  })

  it('useConfigHistory lists versions for a key', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useConfigHistory('export.image'), { wrapper: Wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect((result.current.data ?? []).length).toBeGreaterThanOrEqual(2)
  })
})
