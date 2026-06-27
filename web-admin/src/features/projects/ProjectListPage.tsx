import { useMemo, useState } from 'react'
import { App as AntdApp, Button, Input, Popconfirm, Select, Space, Tag } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import { ProTable } from '@ant-design/pro-components'
import type { ProColumns } from '@ant-design/pro-components'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate } from 'react-router-dom'
import { ApiError } from '@/shared/api/apiError'
import type { Project, ProjectStatus } from '@/shared/api/types'
import { EmptyState } from '@/shared/ui/EmptyState'
import { fmtDate } from '@/shared/time/dayjs'
import { useDeleteProject, useProjects } from './api'
import { ProjectFormModal } from './ProjectFormModal'
import { PROJECT_STATUS_META, PROJECT_STATUS_ORDER } from './status'

const PAGE_SIZE = 10

export function ProjectListPage() {
  const { t } = useTranslation()
  const { message } = AntdApp.useApp()
  const navigate = useNavigate()
  const [page, setPage] = useState(1)
  const [status, setStatus] = useState<ProjectStatus | undefined>(undefined)
  const [q, setQ] = useState('')
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<Project | null>(null)

  const { data, isLoading } = useProjects({
    page,
    page_size: PAGE_SIZE,
    status,
    q: q || undefined,
  })
  const deleteProject = useDeleteProject()
  const rows = useMemo(() => data?.items ?? [], [data])

  const openCreate = (): void => {
    setEditing(null)
    setFormOpen(true)
  }
  const openEdit = (row: Project): void => {
    setEditing(row)
    setFormOpen(true)
  }

  const handleDelete = async (row: Project): Promise<void> => {
    try {
      await deleteProject.mutateAsync(row.id)
      message.success(t('proj.deleted'))
    } catch (err) {
      // RESTRICT：存在巡查记录时后端返回 409 → 行内友好提示（§P3 关键坑）。
      if (err instanceof ApiError && err.code === 'CONFLICT') {
        message.error(t('proj.deleteConflict'))
        return
      }
      throw err
    }
  }

  const columns: ProColumns<Project>[] = [
    {
      title: t('proj.code'),
      dataIndex: 'code',
      width: 160,
      search: false,
      render: (_node, row) => <Link to={`/projects/${row.id}`}>{row.code}</Link>,
    },
    { title: t('proj.name'), dataIndex: 'name', search: false },
    {
      title: t('proj.status'),
      dataIndex: 'status',
      width: 110,
      search: false,
      render: (_node, row) => {
        const meta = PROJECT_STATUS_META[row.status]
        return <Tag color={meta.color}>{meta.label}</Tag>
      },
    },
    {
      title: t('proj.startDate'),
      dataIndex: 'start_date',
      width: 130,
      search: false,
      render: (_node, row) =>
        row.start_date ? (
          <span className="tabular">{fmtDate(`${row.start_date}T00:00:00Z`)}</span>
        ) : (
          <span style={{ color: 'var(--color-ink-muted)' }}>—</span>
        ),
    },
    {
      title: t('proj.endDate'),
      dataIndex: 'end_date',
      width: 130,
      search: false,
      render: (_node, row) =>
        row.end_date ? (
          <span className="tabular">{fmtDate(`${row.end_date}T00:00:00Z`)}</span>
        ) : (
          <span style={{ color: 'var(--color-ink-muted)' }}>—</span>
        ),
    },
    {
      title: t('proj.actions'),
      key: 'actions',
      width: 200,
      search: false,
      render: (_node, row) => (
        <Space size="small">
          <Button type="link" size="small" onClick={() => navigate(`/projects/${row.id}`)}>
            {t('proj.detail')}
          </Button>
          <Button type="link" size="small" onClick={() => openEdit(row)}>
            {t('common.edit')}
          </Button>
          <Popconfirm
            title={t('proj.deleteConfirm')}
            okText={t('common.ok')}
            cancelText={t('common.cancel')}
            onConfirm={() => handleDelete(row)}
          >
            <Button type="link" size="small" danger>
              {t('proj.delete')}
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <div className="app-page">
      <ProTable<Project>
        rowKey="id"
        columns={columns}
        dataSource={rows}
        loading={isLoading}
        search={false}
        cardBordered
        headerTitle={t('proj.title')}
        options={{ reload: false, density: false, setting: false }}
        pagination={{
          current: page,
          pageSize: PAGE_SIZE,
          total: data?.total ?? 0,
          showSizeChanger: false,
          onChange: (next) => setPage(next),
        }}
        toolbar={{
          filter: (
            <Select<ProjectStatus>
              allowClear
              placeholder={t('proj.status')}
              style={{ minWidth: 140 }}
              value={status}
              onChange={(value) => {
                setStatus(value)
                setPage(1)
              }}
              options={PROJECT_STATUS_ORDER.map((s) => ({
                value: s,
                label: PROJECT_STATUS_META[s].label,
              }))}
            />
          ),
          search: (
            <Input.Search
              allowClear
              placeholder={t('proj.searchPlaceholder')}
              style={{ minWidth: 220 }}
              onSearch={(value) => {
                setQ(value.trim())
                setPage(1)
              }}
            />
          ),
          actions: [
            <Button key="new" type="primary" icon={<PlusOutlined />} onClick={openCreate}>
              {t('proj.newProject')}
            </Button>,
          ],
        }}
        locale={{ emptyText: <EmptyState description={t('proj.empty')} /> }}
      />
      <ProjectFormModal open={formOpen} editing={editing} onClose={() => setFormOpen(false)} />
    </div>
  )
}
