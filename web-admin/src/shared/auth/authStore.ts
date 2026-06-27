import { create } from 'zustand'
import { persist, createJSONStorage } from 'zustand/middleware'
import type { AuthMe, AuthTokens, Role, User } from '@/shared/api/types'

/**
 * 认证状态（§3.1 / P0）：token / user / 角色 / 有效权限码。
 * - 仅持久化 access token + user（localStorage）便于刷新保活；refresh token 仅存内存、绝不落盘（§7 / FE-C1）。
 * - 不可变更新：所有 setter 用展开返回新对象/新数组（遵循 coding-style）。
 * - 绝不持久化 refresh token / 密码等长期凭据（§7）。
 */
interface AuthState {
  accessToken: string | null
  refreshToken: string | null
  user: User | null
  roles: Role[]
  permissionCodes: string[]
}

interface AuthActions {
  setTokens: (tokens: AuthTokens) => void
  setMe: (me: AuthMe) => void
  reset: () => void
  hasPermission: (code: string) => boolean
  hasAnyPermission: (codes: string[]) => boolean
}

function emptyState(): AuthState {
  return {
    accessToken: null,
    refreshToken: null,
    user: null,
    roles: [],
    permissionCodes: [],
  }
}

export type AuthStore = AuthState & AuthActions

export const useAuthStore = create<AuthStore>()(
  persist(
    (set, get) => ({
      ...emptyState(),

      setTokens: (tokens) =>
        set((s) => ({
          ...s,
          accessToken: tokens.access_token,
          refreshToken: tokens.refresh_token,
          user: tokens.user,
        })),

      setMe: (me) =>
        set((s) => ({
          ...s,
          user: me.user,
          roles: [...me.roles],
          permissionCodes: [...me.permissions],
        })),

      reset: () => set(() => emptyState()),

      hasPermission: (code) => get().permissionCodes.includes(code),

      hasAnyPermission: (codes) =>
        codes.length === 0 || codes.some((c) => get().permissionCodes.includes(c)),
    }),
    {
      name: 'c5-auth',
      storage: createJSONStorage(() => localStorage),
      // SECURITY (FE-C1): the refresh token is deliberately NOT persisted — it lives
      // in memory only, so a stolen localStorage yields at most a ~15m access token,
      // never the long-lived refresh token. After a full reload the persisted access
      // token keeps the session until it expires, then the user re-authenticates.
      // 权限码/角色不持久化，每次 /auth/me 重新拉取（不信任陈旧权限）。
      partialize: (s) => ({
        accessToken: s.accessToken,
        user: s.user,
      }),
    },
  ),
)
