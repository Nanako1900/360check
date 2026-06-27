import { useState } from 'react'
import { App as AntdApp, Alert, Button, Drawer, Popconfirm, Space, Table, Tag } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { useTranslation } from 'react-i18next'
import type { DictItem, DictType } from '@/shared/api/types'
import { EmptyState } from '@/shared/ui/EmptyState'
import { useDict } from '@/shared/dict/useDict'
import { useUpdateDictItem } from './api'
import { DictItemFormModal } from './DictItemFormModal'

interface DictItemsDrawerProps {
  open: boolean
  dictType: DictType | null
  onClose: () => void
}

/** 颜色色块 + hex 文案（不以颜色为唯一信息载体，§10.2）。 */
function ColorSwatch({ color }: { color: string | null | undefined }) {
  if (!color) return <span style={{ color: 'var(--color-ink-muted)' }}>—</span>
  return (
    <Space size={6}>
      <span
        aria-hidden
        style={{
          display: 'inline-block',
          width: 14,
          height: 14,
          borderRadius: 'var(--radius-sm)',
          background: color,
          border: '1px solid rgba(15,23,42,0.12)',
        }}
      />
      <span className="tabular" style={{ color: 'var(--color-ink-muted)' }}>
        {color}
      </span>
    </Space>
  )
}

export function DictItemsDrawer({ open, dictType, onClose }: DictItemsDrawerProps) {
  const { t } = useTranslation()
  const { message } = AntdApp.useApp()
  const updateItem = useUpdateDictItem()
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<DictItem | null>(null)

  // 复用 useDict（ETag 拉取，含退役项）作为抽屉数据源 → 与 DictTag 共享缓存，变更后自动刷新。
  const code = dictType?.code ?? ''
  const { data, isLoading } = useDict(code, open && code.length > 0)
  const items = data?.items ?? []

  const toggleRetire = async (item: DictItem): Promise<void> => {
    const next = !item.is_active
    await updateItem.mutateAsync({
      id: item.id,
      code,
      body: { is_active: next },
    })
    message.success(next ? t('dict.enabled') : t('dict.retired'))
  }

  const openCreate = (): void => {
    setEditing(null)
    setFormOpen(true)
  }
  const openEdit = (item: DictItem): void => {
    setEditing(item)
    setFormOpen(true)
  }

  const columns: ColumnsType<DictItem> = [
    { title: t('dict.itemCode'), dataIndex: 'code', key: 'code', width: 140 },
    {
      title: t('dict.itemLabel'),
      dataIndex: 'label',
      key: 'label',
      render: (label: string, row) => (
        <Space size={6}>
          <span>{label}</span>
          {row.is_active === false ? (
            <Tag bordered={false} color="default">
              {t('dict.inactive')}
            </Tag>
          ) : null}
        </Space>
      ),
    },
    {
      title: t('dict.itemColor'),
      dataIndex: 'color',
      key: 'color',
      width: 140,
      render: (color: string | null | undefined) => <ColorSwatch color={color} />,
    },
    {
      title: t('dict.itemSort'),
      dataIndex: 'sort_order',
      key: 'sort_order',
      width: 80,
      align: 'right',
    },
    {
      title: t('dict.colActions'),
      key: 'actions',
      width: 160,
      render: (_value, row) => (
        <Space size="small">
          <Button type="link" size="small" onClick={() => openEdit(row)}>
            {t('common.edit')}
          </Button>
          {row.is_active ? (
            <Popconfirm
              title={t('dict.retireConfirm')}
              okText={t('common.ok')}
              cancelText={t('common.cancel')}
              onConfirm={() => toggleRetire(row)}
            >
              <Button type="link" size="small" danger>
                {t('dict.retire')}
              </Button>
            </Popconfirm>
          ) : (
            <Button type="link" size="small" onClick={() => toggleRetire(row)}>
              {t('dict.enable')}
            </Button>
          )}
        </Space>
      ),
    },
  ]

  return (
    <Drawer
      open={open}
      width={680}
      onClose={onClose}
      title={dictType ? `${t('dict.itemsTitle')} · ${dictType.name}` : t('dict.itemsTitle')}
      destroyOnHidden
      extra={
        <Button type="primary" icon={<PlusOutlined />} onClick={openCreate} disabled={!dictType}>
          {t('dict.newItem')}
        </Button>
      }
    >
      <Alert
        type="info"
        showIcon
        banner
        message={t('dict.retireHint')}
        style={{ marginBottom: 'var(--space-5)' }}
      />
      <Table<DictItem>
        rowKey="id"
        size="middle"
        loading={isLoading}
        columns={columns}
        dataSource={items}
        pagination={false}
        locale={{ emptyText: <EmptyState description={t('dict.itemsEmpty')} /> }}
      />
      {dictType ? (
        <DictItemFormModal
          open={formOpen}
          dictType={dictType}
          editing={editing}
          onClose={() => setFormOpen(false)}
        />
      ) : null}
    </Drawer>
  )
}
