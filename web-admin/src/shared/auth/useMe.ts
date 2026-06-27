import { useQuery } from '@tanstack/react-query'
import { http } from '@/shared/api/http'
import { useAuthStore } from './authStore'
import type { AuthMe } from '@/shared/api/types'

export const ME_QUERY_KEY = ['auth', 'me'] as const

/**
 * 拉取 /auth/me 得 roles + effective permission codes，写入 store（驱动动态菜单 + 守卫）。
 * 仅在已有 accessToken 时启用；权限码是动态菜单/路由守卫的唯一权威（§P0）。
 */
export function useMe(enabled = true) {
  const setMe = useAuthStore((s) => s.setMe)
  const accessToken = useAuthStore((s) => s.accessToken)
  return useQuery({
    queryKey: ME_QUERY_KEY,
    queryFn: async () => {
      const me = await http.get<AuthMe>('/auth/me')
      setMe(me)
      return me
    },
    enabled: enabled && Boolean(accessToken),
    staleTime: 5 * 60_000,
  })
}
