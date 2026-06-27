import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { AppRoutes } from '@/routes'
import { sessionBus } from '@/shared/auth/sessionBus'
import { useMe } from '@/shared/auth/useMe'
import { ErrorBoundary } from '@/shared/ui/ErrorBoundary'

export function App() {
  const navigate = useNavigate()

  // 启动时若已有持久化 token，则拉 /auth/me 重建权限码（驱动动态菜单/守卫）。
  useMe()

  // 拦截器在渲染树之外触发的强制登出 → 统一导航到登录页（保留 SPA 路由）。
  useEffect(() => sessionBus.onForceLogout(() => navigate('/login', { replace: true })), [navigate])

  return (
    <ErrorBoundary>
      <AppRoutes />
    </ErrorBoundary>
  )
}
