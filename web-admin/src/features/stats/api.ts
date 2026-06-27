import { useQuery } from '@tanstack/react-query'
import { http } from '@/shared/api/http'
import type { StatsOverview } from '@/shared/api/types'

/**
 * 统计概览查询参数（D2 §4.7）：全可选。时间走 UTC ISO（由 toUtcIso 转换后传入）。
 * 聚合在后端，前端仅渲染（不拉全量、不做大数据聚合）。
 */
export interface StatsOverviewParams {
  project_id?: number
  inspector_id?: number
  /** UTC ISO8601。 */
  from?: string
  to?: string
}

export const statsOverviewKey = (params: StatsOverviewParams) =>
  [
    'stats-overview',
    params.project_id ?? null,
    params.inspector_id ?? null,
    params.from ?? '',
    params.to ?? '',
  ] as const

/** GET /stats/overview（D2 单一权威形状）。 */
export function useStatsOverview(params: StatsOverviewParams, enabled = true) {
  return useQuery<StatsOverview>({
    queryKey: statsOverviewKey(params),
    queryFn: () => {
      const query: Record<string, string | number> = {}
      if (params.project_id) query.project_id = params.project_id
      if (params.inspector_id) query.inspector_id = params.inspector_id
      if (params.from) query.from = params.from
      if (params.to) query.to = params.to
      return http.get<StatsOverview>('/stats/overview', { params: query })
    },
    enabled,
    staleTime: 60_000,
  })
}
