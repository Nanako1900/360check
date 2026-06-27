import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { http } from '@/shared/api/http'
import type {
  Paginated,
  Problem,
  ProblemFeatureCollection,
  ProblemLogCreate,
  ProblemProcessingLog,
  ProblemUpdate,
} from '@/shared/api/types'

/** `GET /problems/map` 过滤参数（§4.7）。时间为 UTC ISO8601。 */
export interface ProblemsMapParams {
  project_id?: number
  inspection_id?: number
  /** type/status/category 为字典项 id（软 FK）。 */
  type?: number
  status?: number
  category?: number
  from?: string
  to?: string
  inspector_id?: number
  limit?: number
}

export const problemsMapKey = (params: ProblemsMapParams) =>
  [
    'problems-map',
    params.project_id ?? null,
    params.inspection_id ?? null,
    params.type ?? null,
    params.status ?? null,
    params.category ?? null,
    params.inspector_id ?? null,
    params.from ?? '',
    params.to ?? '',
    params.limit ?? null,
  ] as const

/** 问题点 GeoJSON FeatureCollection（WGS84，P4 地图聚合层）。 */
export function useProblemsMap(params: ProblemsMapParams, enabled = true) {
  return useQuery<ProblemFeatureCollection>({
    queryKey: problemsMapKey(params),
    queryFn: () => {
      const query: Record<string, string | number> = {}
      if (params.project_id) query.project_id = params.project_id
      if (params.inspection_id) query.inspection_id = params.inspection_id
      if (params.type) query.type = params.type
      if (params.status) query.status = params.status
      if (params.category) query.category = params.category
      if (params.inspector_id) query.inspector_id = params.inspector_id
      if (params.from) query.from = params.from
      if (params.to) query.to = params.to
      if (params.limit) query.limit = params.limit
      return http.get<ProblemFeatureCollection>('/problems/map', { params: query })
    },
    enabled,
    staleTime: 2 * 60_000,
  })
}

// —— 问题列表（§P6 多维筛选）——

/**
 * `GET /problems` 过滤参数（§4.7 / D1）：全 8 维 + 分页。
 * `type`/`status`/`category` 为字典项 id（软 FK）；`from`/`to` 为 UTC ISO8601（由 toUtcIso 转换）。
 */
export interface ProblemListParams {
  project_id?: number
  type?: number
  status?: number
  category?: number
  from?: string
  to?: string
  inspector_id?: number
  /** D1：按巡查会话过滤，巡查详情「该巡查的问题」入口使用。 */
  inspection_id?: number
  page?: number
  page_size?: number
}

export const problemsKey = (params: ProblemListParams) =>
  [
    'problems',
    params.project_id ?? null,
    params.type ?? null,
    params.status ?? null,
    params.category ?? null,
    params.inspector_id ?? null,
    params.inspection_id ?? null,
    params.from ?? '',
    params.to ?? '',
    params.page ?? 1,
    params.page_size ?? 20,
  ] as const
export const problemKey = (id: number) => ['problem', id] as const
export const problemLogsKey = (id: number) => ['problem-logs', id] as const

export function useProblems(params: ProblemListParams, enabled = true) {
  return useQuery<Paginated<Problem>>({
    queryKey: problemsKey(params),
    queryFn: () => {
      const query: Record<string, string | number> = {}
      if (params.project_id) query.project_id = params.project_id
      if (params.type) query.type = params.type
      if (params.status) query.status = params.status
      if (params.category) query.category = params.category
      if (params.inspector_id) query.inspector_id = params.inspector_id
      if (params.inspection_id) query.inspection_id = params.inspection_id
      if (params.from) query.from = params.from
      if (params.to) query.to = params.to
      if (params.page) query.page = params.page
      if (params.page_size) query.page_size = params.page_size
      return http.getList<Problem>('/problems', { params: query })
    },
    enabled,
  })
}

export function useProblem(id: number, enabled = true) {
  return useQuery<Problem>({
    queryKey: problemKey(id),
    queryFn: () => http.get<Problem>(`/problems/${id}`),
    enabled: enabled && id > 0,
  })
}

/**
 * `PUT /problems/{id}`（§P6 / D3 强约束）：改分类/状态/备注等。
 *
 * **D3**：若 body 含新的 `status_item_id`，后端在同一事务内自动追加 `STATUS_CHANGE` 日志。
 * 前端**只发一次 PUT**，**绝不**额外 POST `STATUS_CHANGE`。PUT 成功后失效 problem + 其 logs 查询，
 * 重新拉取 `GET /problems/{id}/logs` 即可看到后端生成的 `STATUS_CHANGE` 行。
 */
export function useUpdateProblem(id: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: ProblemUpdate) => http.put<Problem>(`/problems/${id}`, body),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: problemKey(id) })
      // D3：状态变更后后端原子追加 STATUS_CHANGE → 失效日志查询以拉取该行。
      void qc.invalidateQueries({ queryKey: problemLogsKey(id) })
      void qc.invalidateQueries({ queryKey: ['problems'] })
    },
  })
}

export function useDeleteProblem() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => http.delete<null>(`/problems/${id}`),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['problems'] })
    },
  })
}

/** `GET /problems/{id}/logs`（§P6）：追加式审计时间线（含后端写的 STATUS_CHANGE）。 */
export function useProblemLogs(id: number, enabled = true) {
  return useQuery<Paginated<ProblemProcessingLog>>({
    queryKey: problemLogsKey(id),
    queryFn: () => http.getList<ProblemProcessingLog>(`/problems/${id}/logs`),
    enabled: enabled && id > 0,
  })
}

/**
 * `POST /problems/{id}/logs`（§P6 / D3 强约束）：**仅** COMMENT（纯备注）或 REASSIGN（改派）。
 *
 * 入参类型为 `ProblemLogCreate`，其 `action` 是 `ClientLogAction`（=`'COMMENT' | 'REASSIGN'`，
 * 类型层面已排除 `STATUS_CHANGE`）。状态变更必须走 `useUpdateProblem`（单次 PUT），前端**绝不**
 * 在此构造 `STATUS_CHANGE` payload。
 */
export function useAppendProblemLog(id: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: ProblemLogCreate) =>
      http.post<ProblemProcessingLog>(`/problems/${id}/logs`, body),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: problemLogsKey(id) })
      // REASSIGN 改派会改动问题归属 → 同时失效问题详情。
      void qc.invalidateQueries({ queryKey: problemKey(id) })
    },
  })
}
