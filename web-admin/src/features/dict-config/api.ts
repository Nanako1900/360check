import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { QueryClient } from '@tanstack/react-query'
import { http } from '@/shared/api/http'
import type {
  AppConfig,
  AppConfigUpdate,
  DictItem,
  DictScope,
  DictType,
  DictTypeCreate,
  DictTypeUpdate,
  DictItemCreate,
  DictItemUpdate,
  Paginated,
} from '@/shared/api/types'
import { clearDictCache } from '@/shared/dict/dictCache'
import { dictQueryKey } from '@/shared/dict/useDict'

// —— Query Keys ——
export const dictTypesKey = (scope?: DictScope, isActive?: boolean) =>
  ['dict-types', scope ?? null, isActive ?? null] as const
export const configKey = (key: string) => ['config', key] as const
export const configHistoryKey = (key: string) => ['config-history', key] as const

/**
 * 字典项变更后让 DictTag 自动刷新（§P1 DoD）：失效 ['dict', code] query + 清掉该 code 的内存/会话缓存，
 * 这样下一次 useDict 会重新拉取并按新 ETag 更新。
 */
function invalidateDictByCode(qc: QueryClient, code: string): void {
  clearDictCache()
  void qc.invalidateQueries({ queryKey: dictQueryKey(code) })
}

// —— Dict Types ——

interface DictTypeListParams {
  scope?: DictScope
  is_active?: boolean
}

export function useDictTypes(scope?: DictScope, isActive?: boolean) {
  return useQuery<Paginated<DictType>>({
    queryKey: dictTypesKey(scope, isActive),
    queryFn: () => {
      const params: DictTypeListParams = {}
      if (scope) params.scope = scope
      if (typeof isActive === 'boolean') params.is_active = isActive
      return http.getList<DictType>('/dict/types', { params })
    },
  })
}

export function useCreateDictType() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: DictTypeCreate) => http.post<DictType>('/dict/types', body),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['dict-types'] })
    },
  })
}

export function useUpdateDictType() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: DictTypeUpdate }) =>
      http.put<DictType>(`/dict/types/${id}`, body),
    onSuccess: (updated) => {
      void qc.invalidateQueries({ queryKey: ['dict-types'] })
      invalidateDictByCode(qc, updated.code)
    },
  })
}

export function useDeleteDictType() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => http.delete<null>(`/dict/types/${id}`),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['dict-types'] })
    },
  })
}

// —— Dict Items ——
// 变更字典项必须传入所属 dict_type.code，以便精准失效该字典缓存。

export function useCreateDictItem() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ body }: { code: string; body: DictItemCreate }) =>
      http.post<DictItem>('/dict/items', body),
    onSuccess: (_data, variables) => invalidateDictByCode(qc, variables.code),
  })
}

export function useUpdateDictItem() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: number; code: string; body: DictItemUpdate }) =>
      http.put<DictItem>(`/dict/items/${id}`, body),
    onSuccess: (_data, variables) => invalidateDictByCode(qc, variables.code),
  })
}

export function useDeleteDictItem() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id }: { id: number; code: string }) =>
      http.delete<null>(`/dict/items/${id}`),
    onSuccess: (_data, variables) => invalidateDictByCode(qc, variables.code),
  })
}

// —— App Config ——

export function useConfig(key: string) {
  return useQuery<AppConfig>({
    queryKey: configKey(key),
    queryFn: () => http.get<AppConfig>(`/config/${key}`),
    enabled: key.length > 0,
  })
}

export function useUpdateConfig(key: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: AppConfigUpdate) => http.put<AppConfig>(`/config/${key}`, body),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: configKey(key) })
      void qc.invalidateQueries({ queryKey: configHistoryKey(key) })
    },
  })
}

export function useConfigHistory(key: string, enabled = true) {
  return useQuery<AppConfig[]>({
    queryKey: configHistoryKey(key),
    queryFn: () => http.get<AppConfig[]>(`/config/${key}/history`),
    enabled: enabled && key.length > 0,
  })
}
