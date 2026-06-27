import { Tabs, Typography } from 'antd'
import { useTranslation } from 'react-i18next'
import { UserListPage } from './UserListPage'
import { RoleListPage } from './RoleListPage'

const { Title } = Typography

/** RBAC 管理容器（路由 /rbac 目标）：用户 / 角色 两个标签页。 */
export function RbacPage() {
  const { t } = useTranslation()
  return (
    <div className="app-page">
      <Title level={3} style={{ marginTop: 0 }}>
        {t('rbac.title')}
      </Title>
      <Tabs
        defaultActiveKey="users"
        items={[
          { key: 'users', label: t('rbac.tabUsers'), children: <UserListPage /> },
          { key: 'roles', label: t('rbac.tabRoles'), children: <RoleListPage /> },
        ]}
      />
    </div>
  )
}
