import { useMemo, useState } from 'react'
import { App as AntdApp, Button, Popconfirm, Select, Space, Tag } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import { ProTable } from '@ant-design/pro-components'
import type { ProColumns } from '@ant-design/pro-components'
import { useTranslation } from 'react-i18next'
import type { DictScope, DictType } from '@/shared/api/types'
import { EmptyState } from '@/shared/ui/EmptyState'
import { DICT_SCOPES, scopeLabel } from './scopes'
import { useDeleteDictType, useDictTypes } from './api'
import { DictTypeFormModal } from './DictTypeFormModal'
import { DictItemsDrawer } from './DictItemsDrawer'

export function DictTypeListPage() {
  const { t } = useTranslation()
  const { message } = AntdApp.useApp()
  const [scope, setScope] = useState<DictScope | undefined>(undefined)
  const [isActive, setIsActive] = useState<boolean | undefined>(undefined)
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<DictType | null>(null)
  const [itemsFor, setItemsFor] = useState<DictType | null>(null)

  const { data, isLoading } = useDictTypes(scope, isActive)
  const deleteType = useDeleteDictType()
  const rows = useMemo(() => data?.items ?? [], [data])

  const openCreate = (): void => {
    setEditing(null)
    setFormOpen(true)
  }
  const openEdit = (row: DictType): void => {
    setEditing(row)
    setFormOpen(true)
  }

  const handleDelete = async (row: DictType): Promise<void> => {
    await deleteType.mutateAsync(row.id)
    message.success(t('dict.deleted'))
  }

  const columns: ProColumns<DictType>[] = [
    { title: t('dict.colCode'), dataIndex: 'code', width: 180, copyable: true, search: false },
    { title: t('dict.colName'), dataIndex: 'name', search: false },
    {
      title: t('dict.colScope'),
      dataIndex: 'scope',
      width: 130,
      search: false,
      render: (_node, row) => <Tag bordered={false}>{scopeLabel(row.scope)}</Tag>,
    },
    {
      title: t('dict.colVersion'),
      dataIndex: 'version',
      width: 90,
      align: 'right',
      search: false,
      render: (_node, row) => <span className="tabular">v{row.version}</span>,
    },
    {
      title: t('dict.colActive'),
      dataIndex: 'is_active',
      width: 90,
      search: false,
      render: (_node, row) =>
        row.is_active ? (
          <Tag color="success">{t('dict.active')}</Tag>
        ) : (
          <Tag color="default">{t('dict.inactive')}</Tag>
        ),
    },
    {
      title: t('dict.colActions'),
      key: 'actions',
      width: 240,
      search: false,
      render: (_node, row) => (
        <Space size="small">
          <Button type="link" size="small" onClick={() => setItemsFor(row)}>
            {t('dict.manageItems')}
          </Button>
          <Button type="link" size="small" onClick={() => openEdit(row)}>
            {t('common.save')}
          </Button>
          <Popconfirm
            title={t('dict.deleteTypeConfirm')}
            okText={t('common.ok')}
            cancelText={t('common.cancel')}
            onConfirm={() => handleDelete(row)}
          >
            <Button type="link" size="small" danger>
              {t('dict.deleteTypeTip')}
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <>
      <ProTable<DictType>
        rowKey="id"
        columns={columns}
        dataSource={rows}
        loading={isLoading}
        search={false}
        cardBordered
        pagination={false}
        options={{ reload: false, density: false, setting: false }}
        toolbar={{
          title: t('dict.tabTypes'),
          filter: (
            <Space size="middle" wrap>
              <Select<DictScope>
                allowClear
                placeholder={t('dict.colScope')}
                style={{ minWidth: 160 }}
                value={scope}
                onChange={(value) => setScope(value)}
                options={DICT_SCOPES.map((s) => ({ value: s.value, label: s.label }))}
              />
              <Select<string>
                allowClear
                placeholder={t('dict.colActive')}
                style={{ minWidth: 120 }}
                value={isActive === undefined ? undefined : isActive ? 'active' : 'inactive'}
                onChange={(value) =>
                  setIsActive(value === undefined ? undefined : value === 'active')
                }
                options={[
                  { value: 'active', label: t('dict.active') },
                  { value: 'inactive', label: t('dict.inactive') },
                ]}
              />
            </Space>
          ),
          actions: [
            <Button key="new" type="primary" icon={<PlusOutlined />} onClick={openCreate}>
              {t('dict.newType')}
            </Button>,
          ],
        }}
        locale={{ emptyText: <EmptyState description={t('dict.typesEmpty')} /> }}
      />
      <DictTypeFormModal open={formOpen} editing={editing} onClose={() => setFormOpen(false)} />
      <DictItemsDrawer
        open={itemsFor !== null}
        dictType={itemsFor}
        onClose={() => setItemsFor(null)}
      />
    </>
  )
}
