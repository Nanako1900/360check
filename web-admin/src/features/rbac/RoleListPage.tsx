import { useMemo, useState } from 'react'
import { App as AntdApp, Button, Popconfirm, Space, Tag, Tooltip } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import { ProTable } from '@ant-design/pro-components'
import type { ProColumns } from '@ant-design/pro-components'
import { useTranslation } from 'react-i18next'
import type { Role } from '@/shared/api/types'
import { EmptyState } from '@/shared/ui/EmptyState'
import { useDeleteRole, useRoles } from './api'
import { RoleFormModal } from './RoleFormModal'
import { RolePermissionsDrawer } from './RolePermissionsDrawer'

export function RoleListPage() {
  const { t } = useTranslation()
  const { message } = AntdApp.useApp()
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<Role | null>(null)
  const [permsFor, setPermsFor] = useState<Role | null>(null)

  const { data, isLoading } = useRoles()
  const deleteRole = useDeleteRole()
  const rows = useMemo(() => data?.items ?? [], [data])

  const openCreate = (): void => {
    setEditing(null)
    setFormOpen(true)
  }
  const openEdit = (row: Role): void => {
    setEditing(row)
    setFormOpen(true)
  }

  const handleDelete = async (row: Role): Promise<void> => {
    await deleteRole.mutateAsync(row.id)
    message.success(t('rbac.roleDeleted'))
  }

  const columns: ProColumns<Role>[] = [
    { title: t('rbac.roleCode'), dataIndex: 'code', width: 160, copyable: true, search: false },
    { title: t('rbac.roleName'), dataIndex: 'name', search: false },
    {
      title: t('rbac.roleDescription'),
      dataIndex: 'description',
      search: false,
      render: (_node, row) => row.description ?? '—',
    },
    {
      title: t('rbac.systemRole'),
      dataIndex: 'is_system',
      width: 110,
      search: false,
      render: (_node, row) =>
        row.is_system ? (
          <Tag color="processing">{t('rbac.systemRole')}</Tag>
        ) : (
          <Tag bordered={false}>{t('rbac.customRole')}</Tag>
        ),
    },
    {
      title: t('rbac.roleSortOrder'),
      dataIndex: 'sort_order',
      width: 90,
      align: 'right',
      search: false,
      render: (_node, row) => <span className="tabular">{row.sort_order}</span>,
    },
    {
      title: t('rbac.colActions'),
      key: 'actions',
      width: 240,
      search: false,
      render: (_node, row) => (
        <Space size="small">
          <Button type="link" size="small" onClick={() => openEdit(row)}>
            {t('rbac.edit')}
          </Button>
          <Button type="link" size="small" onClick={() => setPermsFor(row)}>
            {t('rbac.configPermissions')}
          </Button>
          {row.is_system ? (
            <Tooltip title={t('rbac.systemRoleProtected')}>
              <Button type="link" size="small" danger disabled>
                {t('rbac.delete')}
              </Button>
            </Tooltip>
          ) : (
            <Popconfirm
              title={t('rbac.deleteRoleConfirm')}
              okText={t('common.ok')}
              cancelText={t('common.cancel')}
              onConfirm={() => handleDelete(row)}
            >
              <Button type="link" size="small" danger>
                {t('rbac.delete')}
              </Button>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  return (
    <>
      <ProTable<Role>
        rowKey="id"
        columns={columns}
        dataSource={rows}
        loading={isLoading}
        search={false}
        cardBordered
        pagination={false}
        options={{ reload: false, density: false, setting: false }}
        toolbar={{
          title: t('rbac.tabRoles'),
          actions: [
            <Button key="new" type="primary" icon={<PlusOutlined />} onClick={openCreate}>
              {t('rbac.newRole')}
            </Button>,
          ],
        }}
        locale={{ emptyText: <EmptyState description={t('rbac.rolesListEmpty')} /> }}
      />
      <RoleFormModal open={formOpen} editing={editing} onClose={() => setFormOpen(false)} />
      <RolePermissionsDrawer open={permsFor !== null} role={permsFor} onClose={() => setPermsFor(null)} />
    </>
  )
}
