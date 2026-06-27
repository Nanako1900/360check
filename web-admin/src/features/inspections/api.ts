import { useQuery } from '@tanstack/react-query'
import { http } from '@/shared/api/http'
import type { Inspection, InspectionStatus, Paginated, Trajectory } from '@/shared/api/types'

// —— Query Keys ——
export interface InspectionListParams {
  project_id?: number
  inspector_id?: number
  status?: InspectionStatus
  /** UTC ISO8601（由 toUtcIso 转换后传入）。 */
  from?: string
  to?: string
  page?: number
  page_size?: number
}

export const inspectionsKey = (params: InspectionListParams) =>
  [
    'inspections',
    params.project_id ?? null,
    params.inspector_id ?? null,
    params.status ?? '',
    params.from ?? '',
    params.to ?? '',
    params.page ?? 1,
    params.page_size ?? 20,
  ] as const
export const inspectionKey = (id: number) => ['inspection', id] as const
export const trajectoryKey = (id: number) => ['trajectory', id] as const

export function useInspections(params: InspectionListParams, enabled = true) {
  return useQuery<Paginated<Inspection>>({
    queryKey: inspectionsKey(params),
    queryFn: () => {
      const query: Record<string, string | number> = {}
      if (params.project_id) query.project_id = params.project_id
      if (params.inspector_id) query.inspector_id = params.inspector_id
      if (params.status) query.status = params.status
      if (params.from) query.from = params.from
      if (params.to) query.to = params.to
      if (params.page) query.page = params.page
      if (params.page_size) query.page_size = params.page_size
      return http.getList<Inspection>('/inspections', { params: query })
    },
    enabled,
  })
}

export function useInspection(id: number, enabled = true) {
  return useQuery<Inspection>({
    queryKey: inspectionKey(id),
    queryFn: () => http.get<Inspection>(`/inspections/${id}`),
    enabled: enabled && id > 0,
  })
}

/** 巡查轨迹（WGS84 route + points，P4 地图）。坐标在渲染边界经 coordTransform 转 GCJ-02。 */
export function useTrajectory(id: number, enabled = true) {
  return useQuery<Trajectory>({
    queryKey: trajectoryKey(id),
    queryFn: () => http.get<Trajectory>(`/inspections/${id}/trajectory`),
    enabled: enabled && id > 0,
    staleTime: 5 * 60_000,
  })
}
