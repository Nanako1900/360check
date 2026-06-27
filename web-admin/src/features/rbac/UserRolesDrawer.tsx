import { useEffect, useState } from 'react'
import { App as AntdApp, Alert, Button, Checkbox, Drawer, Space, Spin } from 'antd'
import { useTranslation } from 'react-i18next'
import { ApiError } from '@/shared/api/apiError'
import type { User } from '@/shared/api/types'
import { EmptyState } from '@/shared/ui/EmptyState'
import { toCheckedRoleIds } from './permissionTree'
import { useRoles, useSetUserRoles, useUserRoles } from './api'

interface UserRolesDrawerProps {
  open: boolean
  user: User | null
  onClose: () => void
}

/** 编辑用户的角色集合（casbin g-rules）：勾选全部角色，初始勾选取自 GET /users/{id}/roles。 */
export function UserRolesDrawer({ open, user, onClose }: UserRolesDrawerProps) {
  const { t } = useTranslation()
  const { message } = AntdApp.useApp()
  const userId = user?.id ?? 0
  const { data: rolesData, isLoading: rolesLoading } = useRoles()
  const { data: assigned, isLoading: assignedLoading } = useUserRoles(userId, open && userId > 0)
  const setUserRoles = useSetUserRoles(userId)
  const [checked, setChecked] = useState<number[]>([])

  const allRoles = rolesData?.items ?? []

  useEffect(() => {
    if (open && assigned) setChecked(toCheckedRoleIds(assigned.items))
  }, [open, assigned])

  const handleSave = async (): Promise<void> => {
    if (!user) return
    try {
      await setUserRoles.mutateAsync({ role_ids: checked })
      message.success(t('rbac.rolesSaved'))
      onClose()
    } catch (err) {
      if (err instanceof ApiError) message.error(err.message)
    }
  }

  const loading = rolesLoading || assignedLoading

  return (
    <Drawer
      open={open}
      width={480}
      onClose={onClose}
      title={user ? `${t('rbac.rolesTitle')} · ${user.display_name}` : t('rbac.rolesTitle')}
      destroyOnHidden
      extra={
        <Button type="primary" loading={setUserRoles.isPending} onClick={handleSave} disabled={!user}>
          {t('common.save')}
        </Button>
      }
    >
      <Alert
        type="info"
        showIcon
        banner
        message={t('rbac.rolesHint')}
        style={{ marginBottom: 'var(--space-5)' }}
      />
      {loading ? (
        <div style={{ display: 'grid', placeItems: 'center', padding: 'var(--space-8)' }}>
          <Spin />
        </div>
      ) : allRoles.length === 0 ? (
        <EmptyState description={t('rbac.rolesEmpty')} />
      ) : (
        <Checkbox.Group
          value={checked}
          onChange={(values) => setChecked(values as number[])}
          style={{ width: '100%' }}
        >
          <Space direction="vertical" size="middle" style={{ width: '100%' }}>
            {allRoles.map((role) => (
              <Checkbox key={role.id} value={role.id}>
                <span>{role.name}</span>
                <span className="tabular" style={{ marginLeft: 8, color: 'var(--color-ink-muted)' }}>
                  {role.code}
                </span>
              </Checkbox>
            ))}
          </Space>
        </Checkbox.Group>
      )}
    </Drawer>
  )
}
