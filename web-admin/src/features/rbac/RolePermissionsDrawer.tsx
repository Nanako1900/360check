import { useEffect, useState } from 'react'
import { App as AntdApp, Alert, Button, Drawer } from 'antd'
import { useTranslation } from 'react-i18next'
import { ApiError } from '@/shared/api/apiError'
import type { Role } from '@/shared/api/types'
import { toCheckedIds } from './permissionTree'
import { RolePermissionsTree } from './RolePermissionsTree'
import { useRolePermissions, useSetRolePermissions } from './api'

interface RolePermissionsDrawerProps {
  open: boolean
  role: Role | null
  onClose: () => void
}

/** 角色权限配置抽屉（host）：勾选保存 → casbin p-rules。 */
export function RolePermissionsDrawer({ open, role, onClose }: RolePermissionsDrawerProps) {
  const { t } = useTranslation()
  const { message } = AntdApp.useApp()
  const roleId = role?.id ?? 0
  const { data: assigned, isLoading } = useRolePermissions(roleId, open && roleId > 0)
  const setPermissions = useSetRolePermissions(roleId)
  const [checked, setChecked] = useState<number[]>([])

  useEffect(() => {
    if (open && assigned) setChecked(toCheckedIds(assigned.items))
  }, [open, assigned])

  const handleSave = async (): Promise<void> => {
    if (!role) return
    try {
      await setPermissions.mutateAsync({ permission_ids: checked })
      message.success(t('rbac.permissionsSaved'))
      onClose()
    } catch (err) {
      if (err instanceof ApiError) message.error(err.message)
    }
  }

  return (
    <Drawer
      open={open}
      width={520}
      onClose={onClose}
      title={role ? `${t('rbac.permissionsTitle')} · ${role.name}` : t('rbac.permissionsTitle')}
      destroyOnHidden
      extra={
        <Button type="primary" loading={setPermissions.isPending} onClick={handleSave} disabled={!role}>
          {t('common.save')}
        </Button>
      }
    >
      <Alert
        type="info"
        showIcon
        banner
        message={t('rbac.permissionsHint')}
        style={{ marginBottom: 'var(--space-5)' }}
      />
      <RolePermissionsTree
        checkedIds={checked}
        onChange={setChecked}
        loadingChecked={isLoading}
      />
    </Drawer>
  )
}
