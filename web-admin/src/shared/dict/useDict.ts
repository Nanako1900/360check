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
 * - `optional=true`（可选字典，如 project_field 项目自定义字段）：类型未配置时后端返回 404，按
 *   「未配置即空」降级，避免抛错触发全局错误兜底弹窗（与后端「未配置 project_field 即不约束」一致）。
 *   基线字典（problem_*）保持 optional=false：404 仍抛错，让真正的配置缺失浮现。
 */
export async function fetchDict(code: string, optional = false): Promise<CachedDict> {
  const cached = getCachedDict(code)
  const resp = await apiClient.get<Envelope<DictItemsPayload>>(`/dict/types/${code}/items`, {
    params: { include_inactive: true },
    headers: cached ? { 'If-None-Match': `"${cached.contentHash}"` } : undefined,
    validateStatus: (s) => s === 304 || (optional && s === 404) || (s >= 200 && s < 300),
  })

  if (resp.status === 304 && cached) return cached

  // 可选字典 404：类型尚未配置 → 退化为空字典（不写缓存，配置后自动恢复）。
  // 仅 optional 路径会走到这里（非 optional 的 404 已被 validateStatus 拦成抛错）。
  if (resp.status === 404) return cached ?? { items: [], version: 0, contentHash: '' }

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

export function useDict(code: string, enabled = true, optional = false) {
  return useQuery({
    queryKey: dictQueryKey(code),
    queryFn: () => fetchDict(code, optional),
    enabled: enabled && code.length > 0,
    staleTime: 10 * 60_000,
  })
}
