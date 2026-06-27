import { useQuery } from '@tanstack/react-query'
import { apiClient } from '@/shared/api/http'
import type { DictItemsPayload, Envelope } from '@/shared/api/types'
import { getCachedDict, setCachedDict } from './dictCache'
import type { CachedDict } from './dictCache'

export const dictQueryKey = (code: string) => ['dict', code] as const

/**
 * 带 ETag 的字典拉取（§P1 强约束）：
 * - 有本地缓存则发 `If-None-Match: "<content_hash>"`；命中 304 直接用缓存（axios 默认对 304 抛错，
 *   故配 validateStatus 放行 304）。
 * - 200 则更新缓存（含 version/content_hash 以便钉版），**保留 is_active=false 退役项**。
 */
export async function fetchDict(code: string): Promise<CachedDict> {
  const cached = getCachedDict(code)
  const resp = await apiClient.get<Envelope<DictItemsPayload>>(`/dict/types/${code}/items`, {
    params: { include_inactive: true },
    headers: cached ? { 'If-None-Match': `"${cached.contentHash}"` } : undefined,
    validateStatus: (s) => s === 304 || (s >= 200 && s < 300),
  })

  if (resp.status === 304 && cached) return cached

  const payload = resp.data?.data
  if (!payload) {
    if (cached) return cached
    return { items: [], version: 0, contentHash: '' }
  }

  const next: CachedDict = {
    items: payload.items,
    version: payload.version,
    contentHash: payload.content_hash,
  }
  setCachedDict(code, next)
  return next
}

export function useDict(code: string, enabled = true) {
  return useQuery({
    queryKey: dictQueryKey(code),
    queryFn: () => fetchDict(code),
    enabled: enabled && code.length > 0,
    staleTime: 10 * 60_000,
  })
}
