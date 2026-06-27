import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ReactNode } from 'react'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ME_QUERY_KEY } from '@/shared/auth/useMe'
import { useAuthStore } from '@/shared/auth/authStore'
import { db } from '@/mocks/db'
import type { Role, User } from '@/shared/api/types'
import {
  useCreateRole,
  useCreateUser,
  useDeleteRole,
  useDeleteUser,
  usePermissions,
  useResetUserPassword,
  useRolePermissions,
  useRoles,
  useSetRolePermissions,
  useSetUserRoles,
  useUpdateRole,
  useUpdateUser,
  useUserRoles,
  useUsers,
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

function seedUser(): User {
  return {
    id: 1,
    username: 'admin',
    display_name: '系统管理员',
    phone: null,
    email: null,
    avatar_media_id: null,
    is_active: true,
    last_login_at: null,
    created_at: '2026-06-26T00:00:00Z',
    updated_at: '2026-06-26T00:00:00Z',
  }
}

function adminRole(): Role {
  return { id: 1, code: 'admin', name: '管理员', is_system: true, sort_order: 0 }
}

beforeEach(() => {
  useAuthStore.getState().reset()
})

describe('rbac api — users', () => {
  it('useUsers lists seeded users with pagination meta', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useUsers({ page: 1, page_size: 10 }), { wrapper: Wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.total).toBeGreaterThanOrEqual(3)
    expect(result.current.data?.items.some((u) => u.username === 'admin')).toBe(true)
  })

  it('useUsers filters by keyword', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useUsers({ keyword: 'inspector_li' }), { wrapper: Wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.items).toHaveLength(1)
    expect(result.current.data?.items[0].username).toBe('inspector_li')
  })

  it('useCreateUser creates and useUpdateUser edits a user', async () => {
    const { Wrapper } = makeWrapper()
    const create = renderHook(() => useCreateUser(), { wrapper: Wrapper })
    const created = await create.result.current.mutateAsync({
      username: 'new_user',
      password: 'secret123',
      display_name: '新用户',
      is_active: true,
    })
    expect(created.username).toBe('new_user')
    expect(db.users.some((u) => u.username === 'new_user')).toBe(true)

    const update = renderHook(() => useUpdateUser(), { wrapper: Wrapper })
    const updated = await update.result.current.mutateAsync({
      id: created.id,
      body: { display_name: '改名了', is_active: false },
    })
    expect(updated.display_name).toBe('改名了')
    expect(updated.is_active).toBe(false)
  })

  it('useDeleteUser soft-deletes (row no longer in list)', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useDeleteUser(), { wrapper: Wrapper })
    await result.current.mutateAsync(2)
    expect(db.users.some((u) => u.id === 2)).toBe(false)
  })

  it('useResetUserPassword sets a new password for another user', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useResetUserPassword(2), { wrapper: Wrapper })
    await result.current.mutateAsync({ new_password: 'reset12345' })
    expect(db.passwords['inspector_li']).toBe('reset12345')
  })

  it('useUserRoles reads a user current role set and useSetUserRoles replaces it', async () => {
    const { Wrapper } = makeWrapper()
    const read = renderHook(() => useUserRoles(2), { wrapper: Wrapper })
    await waitFor(() => expect(read.result.current.isSuccess).toBe(true))
    expect(read.result.current.data?.items.some((r) => r.code === 'inspector')).toBe(true)

    const set = renderHook(() => useSetUserRoles(2), { wrapper: Wrapper })
    await set.result.current.mutateAsync({ role_ids: [3] })
    expect(db.userRoles[2]).toEqual([3])
  })
})

describe('rbac api — roles & permissions', () => {
  it('useRoles lists seeded roles and usePermissions returns the full catalog', async () => {
    const { Wrapper } = makeWrapper()
    const roles = renderHook(() => useRoles(), { wrapper: Wrapper })
    await waitFor(() => expect(roles.result.current.isSuccess).toBe(true))
    expect(roles.result.current.data?.items.some((r) => r.code === 'admin')).toBe(true)

    const perms = renderHook(() => usePermissions(), { wrapper: Wrapper })
    await waitFor(() => expect(perms.result.current.isSuccess).toBe(true))
    expect(perms.result.current.data?.items.some((p) => p.code === 'user:read')).toBe(true)
  })

  it('useCreateRole, useUpdateRole and useDeleteRole mutate the roles store', async () => {
    const { Wrapper } = makeWrapper()
    const create = renderHook(() => useCreateRole(), { wrapper: Wrapper })
    const created = await create.result.current.mutateAsync({ code: 'viewer', name: '查看者' })
    expect(created.is_system).toBe(false)

    const update = renderHook(() => useUpdateRole(), { wrapper: Wrapper })
    const updated = await update.result.current.mutateAsync({
      id: created.id,
      body: { name: '只读查看者' },
    })
    expect(updated.name).toBe('只读查看者')

    const remove = renderHook(() => useDeleteRole(), { wrapper: Wrapper })
    await remove.result.current.mutateAsync(created.id)
    expect(db.roles.some((r) => r.id === created.id)).toBe(false)
  })

  it('useDeleteRole rejects deleting a system role with CONFLICT', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useDeleteRole(), { wrapper: Wrapper })
    await expect(result.current.mutateAsync(1)).rejects.toMatchObject({ code: 'CONFLICT' })
    expect(db.roles.some((r) => r.id === 1)).toBe(true)
  })

  it('useRolePermissions reads a role current permission set', async () => {
    const { Wrapper } = makeWrapper()
    const { result } = renderHook(() => useRolePermissions(2), { wrapper: Wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.items.some((p) => p.code === 'problem:read')).toBe(true)
  })
})

describe('rbac api — ME invalidation on self-change', () => {
  it('useSetUserRoles invalidates ME_QUERY_KEY when editing own roles', async () => {
    useAuthStore.getState().setMe({ user: seedUser(), roles: [adminRole()], permissions: ['user:read'] })
    const { qc, Wrapper } = makeWrapper()
    const spy = vi.spyOn(qc, 'invalidateQueries')

    const { result } = renderHook(() => useSetUserRoles(1), { wrapper: Wrapper })
    await result.current.mutateAsync({ role_ids: [2] })

    expect(spy).toHaveBeenCalledWith({ queryKey: ME_QUERY_KEY })
  })

  it('useSetUserRoles does NOT invalidate ME when editing another user', async () => {
    useAuthStore.getState().setMe({ user: seedUser(), roles: [adminRole()], permissions: ['user:read'] })
    const { qc, Wrapper } = makeWrapper()
    const spy = vi.spyOn(qc, 'invalidateQueries')

    const { result } = renderHook(() => useSetUserRoles(2), { wrapper: Wrapper })
    await result.current.mutateAsync({ role_ids: [3] })

    expect(spy).not.toHaveBeenCalledWith({ queryKey: ME_QUERY_KEY })
  })

  it('useSetRolePermissions invalidates ME when the role belongs to the current user', async () => {
    useAuthStore.getState().setMe({ user: seedUser(), roles: [adminRole()], permissions: ['user:read'] })
    const { qc, Wrapper } = makeWrapper()
    const spy = vi.spyOn(qc, 'invalidateQueries')

    // 当前用户拥有 role id=1 → 改 role 1 的权限应重拉 ME
    const { result } = renderHook(() => useSetRolePermissions(1), { wrapper: Wrapper })
    await result.current.mutateAsync({ permission_ids: [500, 501] })

    expect(spy).toHaveBeenCalledWith({ queryKey: ME_QUERY_KEY })
  })

  it('useSetRolePermissions does NOT invalidate ME for a role the user lacks', async () => {
    useAuthStore.getState().setMe({ user: seedUser(), roles: [adminRole()], permissions: ['user:read'] })
    const { qc, Wrapper } = makeWrapper()
    const spy = vi.spyOn(qc, 'invalidateQueries')

    // 当前用户没有 role id=3 → 改 role 3 的权限不应重拉 ME
    const { result } = renderHook(() => useSetRolePermissions(3), { wrapper: Wrapper })
    await result.current.mutateAsync({ permission_ids: [500] })

    expect(spy).not.toHaveBeenCalledWith({ queryKey: ME_QUERY_KEY })
  })
})
