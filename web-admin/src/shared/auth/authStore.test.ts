import { beforeEach, describe, expect, it } from 'vitest'
import { useAuthStore } from './authStore'
import type { AuthMe, AuthTokens, User } from '@/shared/api/types'

const user: User = {
  id: 1,
  username: 'admin',
  display_name: '系统管理员',
  is_active: true,
  created_at: '2026-06-26T00:00:00Z',
  updated_at: '2026-06-26T00:00:00Z',
}

const tokens: AuthTokens = { access_token: 'a', refresh_token: 'r', expires_in: 900, user }

beforeEach(() => {
  useAuthStore.getState().reset()
})

describe('authStore', () => {
  it('setTokens stores access/refresh/user', () => {
    useAuthStore.getState().setTokens(tokens)
    const s = useAuthStore.getState()
    expect(s.accessToken).toBe('a')
    expect(s.refreshToken).toBe('r')
    expect(s.user?.username).toBe('admin')
  })

  it('setMe copies roles/permissions into new arrays (no input mutation)', () => {
    const me: AuthMe = {
      user,
      roles: [{ id: 1, code: 'admin', name: '管理员', is_system: true, sort_order: 0 }],
      permissions: ['project:read', 'stats:read'],
    }
    useAuthStore.getState().setMe(me)
    const s = useAuthStore.getState()
    expect(s.permissionCodes).toEqual(['project:read', 'stats:read'])
    expect(s.permissionCodes).not.toBe(me.permissions)
    expect(s.roles).not.toBe(me.roles)
  })

  it('hasPermission / hasAnyPermission evaluate the current codes', () => {
    useAuthStore.getState().setMe({ user, roles: [], permissions: ['a', 'b'] })
    const s = useAuthStore.getState()
    expect(s.hasPermission('a')).toBe(true)
    expect(s.hasPermission('z')).toBe(false)
    expect(s.hasAnyPermission(['z', 'b'])).toBe(true)
    expect(s.hasAnyPermission(['z'])).toBe(false)
    expect(s.hasAnyPermission([])).toBe(true)
  })

  it('reset clears everything', () => {
    useAuthStore.getState().setTokens(tokens)
    useAuthStore.getState().setMe({ user, roles: [], permissions: ['a'] })
    useAuthStore.getState().reset()
    const s = useAuthStore.getState()
    expect(s.accessToken).toBeNull()
    expect(s.user).toBeNull()
    expect(s.roles).toEqual([])
    expect(s.permissionCodes).toEqual([])
  })
})
