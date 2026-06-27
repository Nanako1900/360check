import type { ReactNode } from 'react'
import { Navigate, useLocation } from 'react-router-dom'
import { useAuthStore } from './authStore'
import { Forbidden } from '@/shared/ui/Forbidden'

interface RequireAuthProps {
  children: ReactNode
}

/** 无 token → 跳登录（携带 from 以便回跳）。真正鉴权在后端，这里仅是 UI 守卫（§P0/§7）。 */
export function RequireAuth({ children }: RequireAuthProps) {
  const accessToken = useAuthStore((s) => s.accessToken)
  const location = useLocation()
  if (!accessToken) {
    return (
      <Navigate to="/login" replace state={{ from: `${location.pathname}${location.search}` }} />
    )
  }
  return <>{children}</>
}

interface RequirePermissionProps {
  /** 单个或任一满足即可（前端仅 UI 隐藏；后端仍可能返回 FORBIDDEN）。 */
  code: string | string[]
  children: ReactNode
}

export function RequirePermission({ code, children }: RequirePermissionProps) {
  const codes = Array.isArray(code) ? code : [code]
  const ok = useAuthStore(
    (s) => codes.length === 0 || codes.some((c) => s.permissionCodes.includes(c)),
  )
  if (!ok) return <Forbidden />
  return <>{children}</>
}
