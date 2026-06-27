import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { QueryClient } from '@tanstack/react-query'
import { http } from '@/shared/api/http'
import { useAuthStore } from '@/shared/auth/authStore'
import { ME_QUERY_KEY } from '@/shared/auth/useMe'
import type {
  Paginated,
  Permission,
  ResetPasswordRequest,
  Role,
  RoleCreate,
  RoleUpdate,
  SetRolePermissionsRequest,
  SetUserRolesRequest,
  User,
  UserCreate,
  UserUpdate,
} from '@/shared/api/types'

// —— Query Keys ——
export interface UserListParams {
  page?: number
  page_size?: number
  keyword?: string
}

export const usersKey = (params: UserListParams) =>
  ['rbac', 'users', params.page ?? 1, params.page_size ?? 20, params.keyword ?? ''] as const
export const userRolesKey = (id: number) => ['rbac', 'user-roles', id] as const
export const rolesKey = ['rbac', 'roles'] as const
export const rolePermissionsKey = (id: number) => ['rbac', 'role-permissions', id] as const
export const permissionsKey = ['rbac', 'permissions'] as const

/**
 * 改了「当前用户自己」的角色/权限后，重拉 /auth/me（§P2 关键坑）：
 * 角色/权限码是动态菜单与守卫的唯一权威，自我变更必须即时刷新，避免菜单与实际权限脱节。
 */
function invalidateMe(qc: QueryClient): void {
  void qc.invalidateQueries({ queryKey: ME_QUERY_KEY })
}

/** 当前登录用户 id（用于判断「改的是不是自己」）。 */
function currentUserId(): number | null {
  return useAuthStore.getState().user?.id ?? null
}

/** 当前登录用户拥有的角色 id 集合（用于判断角色权限变更是否影响自己）。 */
function currentUserRoleIds(): number[] {
  return useAuthStore.getState().roles.map((r) => r.id)
}

// —— Users ——

export function useUsers(params: UserListParams) {
  return useQuery<Paginated<User>>({
    queryKey: usersKey(params),
    queryFn: () => {
      const query: Record<string, string | number> = {}
      if (params.page) query.page = params.page
      if (params.page_size) query.page_size = params.page_size
      if (params.keyword) query.keyword = params.keyword
      return http.getList<User>('/users', { params: query })
    },
  })
}

export function useCreateUser() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: UserCreate) => http.post<User>('/users', body),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['rbac', 'users'] })
    },
  })
}

export function useUpdateUser() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: UserUpdate }) =>
      http.put<User>(`/users/${id}`, body),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['rbac', 'users'] })
    },
  })
}

export function useDeleteUser() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => http.delete<null>(`/users/${id}`),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['rbac', 'users'] })
    },
  })
}

export function useResetUserPassword(id: number) {
  return useMutation({
    mutationFn: (body: ResetPasswordRequest) =>
      http.put<null>(`/users/${id}/password`, body),
  })
}

export function useUserRoles(id: number, enabled = true) {
  return useQuery<Paginated<Role>>({
    queryKey: userRolesKey(id),
    queryFn: () => http.getList<Role>(`/users/${id}/roles`),
    enabled: enabled && id > 0,
  })
}

export function useSetUserRoles(id: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: SetUserRolesRequest) =>
      http.put<Role[]>(`/users/${id}/roles`, body),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: userRolesKey(id) })
      void qc.invalidateQueries({ queryKey: ['rbac', 'users'] })
      // 改的是自己 → 重拉 /auth/me 刷新菜单/守卫
      if (currentUserId() === id) invalidateMe(qc)
    },
  })
}

// —— Roles ——

export function useRoles() {
  return useQuery<Paginated<Role>>({
    queryKey: rolesKey,
    queryFn: () => http.getList<Role>('/roles'),
  })
}

export function useCreateRole() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: RoleCreate) => http.post<Role>('/roles', body),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: rolesKey })
    },
  })
}

export function useUpdateRole() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: RoleUpdate }) =>
      http.put<Role>(`/roles/${id}`, body),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: rolesKey })
    },
  })
}

export function useDeleteRole() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => http.delete<null>(`/roles/${id}`),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: rolesKey })
    },
  })
}

export function useRolePermissions(id: number, enabled = true) {
  return useQuery<Paginated<Permission>>({
    queryKey: rolePermissionsKey(id),
    queryFn: () => http.getList<Permission>(`/roles/${id}/permissions`),
    enabled: enabled && id > 0,
  })
}

export function useSetRolePermissions(id: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: SetRolePermissionsRequest) =>
      http.put<Permission[]>(`/roles/${id}/permissions`, body),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: rolePermissionsKey(id) })
      // 改的角色含当前用户 → 重拉 /auth/me 刷新菜单/守卫
      if (currentUserRoleIds().includes(id)) invalidateMe(qc)
    },
  })
}

// —— Permissions catalog ——

export function usePermissions() {
  return useQuery<Paginated<Permission>>({
    queryKey: permissionsKey,
    queryFn: () => http.getList<Permission>('/permissions'),
  })
}
