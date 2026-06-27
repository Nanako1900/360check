import { useMemo, useState } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { ProLayout } from '@ant-design/pro-components'
import type { MenuDataItem } from '@ant-design/pro-components'
import { Dropdown, Space } from 'antd'
import { KeyOutlined, LogoutOutlined, UserOutlined } from '@ant-design/icons'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/shared/auth/authStore'
import { useLogout } from '@/shared/auth/useLogout'
import { filterMenu, MENU } from '@/routes/menu'
import type { AppMenuItem } from '@/routes/menu'
import { ChangePasswordModal } from '@/features/login/ChangePasswordModal'

function toMenuData(items: AppMenuItem[], t: (k: string) => string): MenuDataItem[] {
  return items.map((item) => ({
    path: item.path,
    name: t(item.name),
    icon: item.icon,
    children: item.children ? toMenuData(item.children, t) : undefined,
  }))
}

/** ProLayout 外壳：动态菜单（按权限码过滤）+ 用户菜单（改密/登出）+ 路由出口。 */
export function AppLayout() {
  const { t } = useTranslation()
  const location = useLocation()
  const navigate = useNavigate()
  const user = useAuthStore((s) => s.user)
  const permissionCodes = useAuthStore((s) => s.permissionCodes)
  const logout = useLogout()
  const [pwOpen, setPwOpen] = useState(false)

  const menuData = useMemo(() => {
    const can = (code: string) => permissionCodes.includes(code)
    return toMenuData(filterMenu(MENU, can), t)
  }, [permissionCodes, t])

  const handleLogout = (): void => {
    logout.mutate(undefined, {
      onSettled: () => navigate('/login', { replace: true }),
    })
  }

  return (
    <>
      <ProLayout
        title={t('app.shortTitle')}
        logo={false}
        layout="side"
        navTheme="light"
        fixSiderbar
        fixedHeader
        siderWidth={232}
        location={{ pathname: location.pathname }}
        route={{ path: '/', routes: menuData }}
        menuItemRender={(item, dom) => (
          <a
            onClick={() => {
              if (item.path && item.path !== location.pathname) navigate(item.path)
            }}
          >
            {dom}
          </a>
        )}
        avatarProps={{
          icon: <UserOutlined />,
          title: user?.display_name ?? user?.username ?? '',
          size: 'small',
          render: (_props, dom) => (
            <Dropdown
              menu={{
                items: [
                  { key: 'password', icon: <KeyOutlined />, label: t('common.changePassword') },
                  { type: 'divider' },
                  {
                    key: 'logout',
                    icon: <LogoutOutlined />,
                    label: t('common.logout'),
                    danger: true,
                  },
                ],
                onClick: ({ key }) => {
                  if (key === 'password') setPwOpen(true)
                  if (key === 'logout') handleLogout()
                },
              }}
            >
              <Space style={{ cursor: 'pointer' }}>{dom}</Space>
            </Dropdown>
          ),
        }}
      >
        <Outlet />
      </ProLayout>
      <ChangePasswordModal
        open={pwOpen}
        onClose={() => setPwOpen(false)}
        onSuccess={handleLogout}
      />
    </>
  )
}
