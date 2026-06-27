import { useMemo } from 'react'
import { Button, Card, Space, Table, Tag } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import type { Inspection, User } from '@/shared/api/types'
import { EmptyState } from '@/shared/ui/EmptyState'
import { fmt } from '@/shared/time/dayjs'
import { formatDuration, formatMileageKm } from '@/shared/util/format'
import { useUsers } from '@/features/rbac/api'
import { useInspections } from '@/features/inspections/api'
import { INSPECTION_STATUS_META } from '@/features/inspections/status'

interface ProjectInspectionsCardProps {
  projectId: number
}

/** 项目详情内的巡查记录卡片（§P3）：只读列表 + 跳转完整巡查筛选页入口。 */
export function ProjectInspectionsCard({ projectId }: ProjectInspectionsCardProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { data, isLoading } = useInspections({ project_id: projectId, page: 1, page_size: 50 })
  const rows = useMemo(() => data?.items ?? [], [data])

  const { data: usersData } = useUsers({ page: 1, page_size: 200 })
  const userById = useMemo(() => {
    const map = new Map<number, User>()
    for (const u of usersData?.items ?? []) map.set(u.id, u)
    return map
  }, [usersData])

  const columns: ColumnsType<Inspection> = [
    {
      title: t('inspection.inspector'),
      dataIndex: 'inspector_id',
      key: 'inspector_id',
      width: 120,
      render: (value: number) => {
        const u = userById.get(value)
        return u ? u.display_name || u.username : `#${value}`
      },
    },
    {
      title: t('inspection.startedAt'),
      dataIndex: 'started_at',
      key: 'started_at',
      width: 150,
      render: (value: string) => <span className="tabular">{fmt(value)}</span>,
    },
    {
      title: t('inspection.duration'),
      dataIndex: 'duration_seconds',
      key: 'duration_seconds',
      width: 110,
      align: 'right',
      render: (value: number, row) =>
        row.status === 'IN_PROGRESS' ? (
          <span style={{ color: 'var(--color-ink-muted)' }}>{t('inspection.inProgress')}</span>
        ) : (
          <span className="tabular">{formatDuration(value)}</span>
        ),
    },
    {
      title: t('inspection.mileage'),
      dataIndex: 'mileage_meters',
      key: 'mileage_meters',
      width: 100,
      align: 'right',
      render: (value: number, row) =>
        row.status === 'IN_PROGRESS' ? (
          <span style={{ color: 'var(--color-ink-muted)' }}>—</span>
        ) : (
          <span className="tabular">{formatMileageKm(value)}</span>
        ),
    },
    {
      title: t('inspection.status'),
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (_value, row) => {
        const meta = INSPECTION_STATUS_META[row.status]
        return <Tag color={meta.color}>{meta.label}</Tag>
      },
    },
    {
      title: t('inspection.actions'),
      key: 'actions',
      width: 90,
      render: (_value, row) => (
        <Button type="link" size="small" onClick={() => navigate(`/inspections/${row.id}`)}>
          {t('inspection.detail')}
        </Button>
      ),
    },
  ]

  return (
    <Card
      bordered
      title={t('proj.relatedInspections')}
      extra={
        <Space>
          <Button
            type="link"
            onClick={() => navigate(`/inspections?project_id=${projectId}`)}
          >
            {t('proj.viewInspections')}
          </Button>
        </Space>
      }
    >
      <Table<Inspection>
        rowKey="id"
        size="middle"
        loading={isLoading}
        columns={columns}
        dataSource={rows}
        pagination={false}
        locale={{ emptyText: <EmptyState description={t('proj.inspectionsEmpty')} /> }}
      />
    </Card>
  )
}
