import { useMemo, useState } from 'react'
import { App as AntdApp, Button, Input, Popconfirm, Space, Tag } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import { ProTable } from '@ant-design/pro-components'
import type { ProColumns } from '@ant-design/pro-components'
import { useTranslation } from 'react-i18next'
import type { User } from '@/shared/api/types'
import { EmptyState } from '@/shared/ui/EmptyState'
import { fmt } from '@/shared/time/dayjs'
import { useDeleteUser, useUsers } from './api'
import { UserFormModal } from './UserFormModal'
import { UserRolesDrawer } from './UserRolesDrawer'
import { ResetPasswordModal } from './ResetPasswordModal'

const PAGE_SIZE = 10

export function UserListPage() {
  const { t } = useTranslation()
  const { message } = AntdApp.useApp()
  const [page, setPage] = useState(1)
  const [keyword, setKeyword] = useState('')
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<User | null>(null)
  const [rolesFor, setRolesFor] = useState<User | null>(null)
  const [resetFor, setResetFor] = useState<User | null>(null)

  const { data, isLoading } = useUsers({ page, page_size: PAGE_SIZE, keyword: keyword || undefined })
  const deleteUser = useDeleteUser()
  const rows = useMemo(() => data?.items ?? [], [data])

  const openCreate = (): void => {
    setEditing(null)
    setFormOpen(true)
  }
  const openEdit = (row: User): void => {
    setEditing(row)
    setFormOpen(true)
  }

  const handleDelete = async (row: User): Promise<void> => {
    await deleteUser.mutateAsync(row.id)
    message.success(t('rbac.userDeleted'))
  }

  const columns: ProColumns<User>[] = [
    { title: t('rbac.colUsername'), dataIndex: 'username', width: 160, copyable: true, search: false },
    { title: t('rbac.colDisplayName'), dataIndex: 'display_name', search: false },
    {
      title: t('rbac.colPhone'),
      dataIndex: 'phone',
      width: 140,
      search: false,
      render: (_node, row) => row.phone ?? '—',
    },
    {
      title: t('rbac.colActive'),
      dataIndex: 'is_active',
      width: 90,
      search: false,
      render: (_node, row) =>
        row.is_active ? (
          <Tag color="success">{t('rbac.active')}</Tag>
        ) : (
          <Tag color="default">{t('rbac.inactive')}</Tag>
        ),
    },
    {
      title: t('rbac.colLastLogin'),
      dataIndex: 'last_login_at',
      width: 160,
      search: false,
      render: (_node, row) =>
        row.last_login_at ? (
          <span className="tabular">{fmt(row.last_login_at)}</span>
        ) : (
          <span style={{ color: 'var(--color-ink-muted)' }}>{t('rbac.neverLoggedIn')}</span>
        ),
    },
    {
      title: t('rbac.colActions'),
      key: 'actions',
      width: 280,
      search: false,
      render: (_node, row) => (
        <Space size="small">
          <Button type="link" size="small" onClick={() => openEdit(row)}>
            {t('rbac.edit')}
          </Button>
          <Button type="link" size="small" onClick={() => setRolesFor(row)}>
            {t('rbac.assignRoles')}
          </Button>
          <Button type="link" size="small" onClick={() => setResetFor(row)}>
            {t('rbac.resetPassword')}
          </Button>
          <Popconfirm
            title={t('rbac.deleteUserConfirm')}
            okText={t('common.ok')}
            cancelText={t('common.cancel')}
            onConfirm={() => handleDelete(row)}
          >
            <Button type="link" size="small" danger>
              {t('rbac.delete')}
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <>
      <ProTable<User>
        rowKey="id"
        columns={columns}
        dataSource={rows}
        loading={isLoading}
        search={false}
        cardBordered
        options={{ reload: false, density: false, setting: false }}
        pagination={{
          current: page,
          pageSize: PAGE_SIZE,
          total: data?.total ?? 0,
          showSizeChanger: false,
          onChange: (next) => setPage(next),
        }}
        toolbar={{
          title: t('rbac.tabUsers'),
          search: (
            <Input.Search
              allowClear
              placeholder={t('rbac.searchUser')}
              style={{ minWidth: 220 }}
              onSearch={(value) => {
                setKeyword(value.trim())
                setPage(1)
              }}
            />
          ),
          actions: [
            <Button key="new" type="primary" icon={<PlusOutlined />} onClick={openCreate}>
              {t('rbac.newUser')}
            </Button>,
          ],
        }}
        locale={{ emptyText: <EmptyState description={t('rbac.usersEmpty')} /> }}
      />
      <UserFormModal open={formOpen} editing={editing} onClose={() => setFormOpen(false)} />
      <UserRolesDrawer open={rolesFor !== null} user={rolesFor} onClose={() => setRolesFor(null)} />
      <ResetPasswordModal open={resetFor !== null} user={resetFor} onClose={() => setResetFor(null)} />
    </>
  )
}
