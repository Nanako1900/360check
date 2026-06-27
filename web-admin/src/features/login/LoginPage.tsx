import { LoginForm, ProFormText } from '@ant-design/pro-components'
import { LockOutlined, UserOutlined } from '@ant-design/icons'
import { App as AntdApp } from 'antd'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { useLocation, useNavigate } from 'react-router-dom'
import { http } from '@/shared/api/http'
import { ApiError } from '@/shared/api/apiError'
import { useAuthStore } from '@/shared/auth/authStore'
import { useLogin } from '@/shared/auth/useLogin'
import { ME_QUERY_KEY } from '@/shared/auth/useMe'
import type { AuthMe, LoginRequest } from '@/shared/api/types'
import './login.css'

interface LocationState {
  from?: string
}

export function LoginPage() {
  const { t } = useTranslation()
  const login = useLogin()
  const navigate = useNavigate()
  const location = useLocation()
  const queryClient = useQueryClient()
  const setMe = useAuthStore((s) => s.setMe)
  const { message } = AntdApp.useApp()

  const from = (location.state as LocationState | null)?.from ?? '/'

  const handleFinish = async (values: LoginRequest): Promise<boolean> => {
    try {
      await login.mutateAsync(values)
      // 登录后立即拉 /auth/me 得权限码（驱动菜单/守卫），并写入 query 缓存供复用
      const me = await queryClient.fetchQuery({
        queryKey: ME_QUERY_KEY,
        queryFn: () => http.get<AuthMe>('/auth/me'),
      })
      setMe(me)
      message.success(t('login.success'))
      navigate(from, { replace: true })
      return true
    } catch (err) {
      message.error(err instanceof ApiError ? err.message : t('common.retry'))
      return false
    }
  }

  return (
    <div className="login-shell">
      <aside className="login-brand" aria-hidden="true">
        <div>
          <p className="login-brand__kicker">C5 · Inspection Console</p>
          <h1 className="login-brand__title">360 相机巡查标注系统</h1>
          <p className="login-brand__desc">
            项目、巡查轨迹、问题工单、全景照片与数据统计，集中在一处管理。WGS84
            持久化、腾讯地图渲染、Excel 异步导出。
          </p>
        </div>
        <div className="login-brand__meta">
          <div>
            <b className="num">8</b>
            <span>核心模块</span>
          </div>
          <div>
            <b className="num">360°</b>
            <span>全景标注</span>
          </div>
          <div>
            <b className="num">WGS84</b>
            <span>坐标基准</span>
          </div>
        </div>
      </aside>

      <main className="login-panel">
        <div className="login-card">
          <LoginForm<LoginRequest>
            title={t('login.title')}
            subTitle={t('login.subtitle')}
            onFinish={handleFinish}
            loading={login.isPending}
            submitter={{ searchConfig: { submitText: t('login.submit') } }}
          >
            <ProFormText
              name="username"
              fieldProps={{ size: 'large', prefix: <UserOutlined />, autoComplete: 'username' }}
              placeholder={t('login.usernamePlaceholder')}
              rules={[{ required: true, message: t('login.usernameRequired') }]}
            />
            <ProFormText.Password
              name="password"
              fieldProps={{
                size: 'large',
                prefix: <LockOutlined />,
                autoComplete: 'current-password',
              }}
              placeholder={t('login.passwordPlaceholder')}
              rules={[{ required: true, message: t('login.passwordRequired') }]}
            />
          </LoginForm>
        </div>
      </main>
    </div>
  )
}
