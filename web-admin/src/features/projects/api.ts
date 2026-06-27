import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { http } from '@/shared/api/http'
import type {
  InspectionTask,
  InspectionTaskCreate,
  InspectionTaskUpdate,
  Paginated,
  Project,
  ProjectCreate,
  ProjectStatus,
  ProjectUpdate,
  TaskStatus,
} from '@/shared/api/types'

// —— Query Keys ——
export interface ProjectListParams {
  status?: ProjectStatus
  q?: string
  page?: number
  page_size?: number
}

export interface TaskListParams {
  project_id?: number
  status?: TaskStatus
  assignee_id?: number
}

export const projectsKey = (params: ProjectListParams) =>
  ['projects', params.status ?? '', params.q ?? '', params.page ?? 1, params.page_size ?? 20] as const
export const projectKey = (id: number) => ['project', id] as const
export const tasksKey = (params: TaskListParams) =>
  ['tasks', params.project_id ?? null, params.status ?? '', params.assignee_id ?? null] as const

// —— Projects ——

export function useProjects(params: ProjectListParams) {
  return useQuery<Paginated<Project>>({
    queryKey: projectsKey(params),
    queryFn: () => {
      const query: Record<string, string | number> = {}
      if (params.status) query.status = params.status
      if (params.q) query.q = params.q
      if (params.page) query.page = params.page
      if (params.page_size) query.page_size = params.page_size
      return http.getList<Project>('/projects', { params: query })
    },
  })
}

export function useProject(id: number, enabled = true) {
  return useQuery<Project>({
    queryKey: projectKey(id),
    queryFn: () => http.get<Project>(`/projects/${id}`),
    enabled: enabled && id > 0,
  })
}

export function useCreateProject() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: ProjectCreate) => http.post<Project>('/projects', body),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['projects'] })
    },
  })
}

export function useUpdateProject() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: ProjectUpdate }) =>
      http.put<Project>(`/projects/${id}`, body),
    onSuccess: (updated) => {
      void qc.invalidateQueries({ queryKey: ['projects'] })
      void qc.invalidateQueries({ queryKey: projectKey(updated.id) })
    },
  })
}

export function useDeleteProject() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => http.delete<null>(`/projects/${id}`),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['projects'] })
    },
  })
}

// —— Tasks（项目子资源）——

export function useTasks(params: TaskListParams, enabled = true) {
  return useQuery<Paginated<InspectionTask>>({
    queryKey: tasksKey(params),
    queryFn: () => {
      const query: Record<string, string | number> = {}
      if (params.project_id) query.project_id = params.project_id
      if (params.status) query.status = params.status
      if (params.assignee_id) query.assignee_id = params.assignee_id
      return http.getList<InspectionTask>('/tasks', { params: query })
    },
    enabled,
  })
}

export function useCreateTask() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: InspectionTaskCreate) => http.post<InspectionTask>('/tasks', body),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['tasks'] })
    },
  })
}

export function useUpdateTask() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: InspectionTaskUpdate }) =>
      http.put<InspectionTask>(`/tasks/${id}`, body),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['tasks'] })
    },
  })
}

export function useDeleteTask() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => http.delete<null>(`/tasks/${id}`),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['tasks'] })
    },
  })
}
