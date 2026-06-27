import { useMemo, useState } from 'react'
import { Button, DatePicker, Select, Space, Tag } from 'antd'
import { ProTable } from '@ant-design/pro-components'
import type { ProColumns } from '@ant-design/pro-components'
import { useTranslation } from 'react-i18next'
import { useNavigate, useSearchParams } from 'react-router-dom'
import type { Dayjs } from 'dayjs'
import type { Inspection, InspectionStatus, Project, User } from '@/shared/api/types'
import { EmptyState } from '@/shared/ui/EmptyState'
import { fmt, toUtcIso } from '@/shared/time/dayjs'
import { formatDuration, formatMileageKm } from '@/shared/util/format'
import { useUsers } from '@/features/rbac/api'
import { useProjects } from '@/features/projects/api'
import { ExportButton } from '@/features/exports/ExportButton'
import { useInspections } from './api'
import { INSPECTION_STATUS_META, INSPECTION_STATUS_ORDER } from './status'

const PAGE_SIZE = 10

export function InspectionListPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const initialProjectId = searchParams.get('project_id')

  const [page, setPage] = useState(1)
  const [projectId, setProjectId] = useState<number | undefined>(
    initialProjectId ? Number(initialProjectId) : undefined,
  )
  const [inspectorId, setInspectorId] = useState<number | undefined>(undefined)
  const [status, setStatus] = useState<InspectionStatus | undefined>(undefined)
  const [range, setRange] = useState<[Dayjs, Dayjs] | null>(null)

  const from = range?.[0] ? toUtcIso(range[0]) : undefined
  const to = range?.[1] ? toUtcIso(range[1]) : undefined

  const { data, isLoading } = useInspections({
    project_id: projectId,
    inspector_id: inspectorId,
    status,
    from,
    to,
    page,
    page_size: PAGE_SIZE,
  })
  const rows = useMemo(() => data?.items ?? [], [data])

  const { data: projectsData } = useProjects({ page: 1, page_size: 200 })
  const projectById = useMemo(() => {
    const map = new Map<number, Project>()
    for (const p of projectsData?.items ?? []) map.set(p.id, p)
    return map
  }, [projectsData])
  const projectOptions = (projectsData?.items ?? []).map((p) => ({ value: p.id, label: p.name }))

  const { data: usersData } = useUsers({ page: 1, page_size: 200 })
  const userById = useMemo(() => {
    const map = new Map<number, User>()
    for (const u of usersData?.items ?? []) map.set(u.id, u)
    return map
  }, [usersData])
  const inspectorOptions = (usersData?.items ?? []).map((u) => ({
    value: u.id,
    label: u.display_name || u.username,
  }))

  const resetPage = (): void => setPage(1)

  const columns: ProColumns<Inspection>[] = [
    {
      title: t('inspection.project'),
      dataIndex: 'project_id',
      width: 160,
      search: false,
      render: (_node, row) => projectById.get(row.project_id)?.name ?? `#${row.project_id}`,
    },
    {
      title: t('inspection.inspector'),
      dataIndex: 'inspector_id',
      width: 120,
      search: false,
      render: (_node, row) => {
        const u = userById.get(row.inspector_id)
        return u ? u.display_name || u.username : `#${row.inspector_id}`
      },
    },
    {
      title: t('inspection.startedAt'),
      dataIndex: 'started_at',
      width: 150,
      search: false,
      render: (_node, row) => <span className="tabular">{fmt(row.started_at)}</span>,
    },
    {
      title: t('inspection.endedAt'),
      dataIndex: 'ended_at',
      width: 150,
      search: false,
      render: (_node, row) =>
        row.ended_at ? (
          <span className="tabular">{fmt(row.ended_at)}</span>
        ) : (
          <span style={{ color: 'var(--color-ink-muted)' }}>—</span>
        ),
    },
    {
      title: t('inspection.duration'),
      dataIndex: 'duration_seconds',
      width: 120,
      align: 'right',
      search: false,
      render: (_node, row) =>
        row.status === 'IN_PROGRESS' ? (
          <span style={{ color: 'var(--color-ink-muted)' }}>{t('inspection.inProgress')}</span>
        ) : (
          <span className="tabular">{formatDuration(row.duration_seconds)}</span>
        ),
    },
    {
      title: t('inspection.mileage'),
      dataIndex: 'mileage_meters',
      width: 110,
      align: 'right',
      search: false,
      render: (_node, row) =>
        row.status === 'IN_PROGRESS' ? (
          <span style={{ color: 'var(--color-ink-muted)' }}>—</span>
        ) : (
          <span className="tabular">{formatMileageKm(row.mileage_meters)}</span>
        ),
    },
    {
      title: t('inspection.pointCount'),
      dataIndex: 'point_count',
      width: 100,
      align: 'right',
      search: false,
      render: (_node, row) => <span className="tabular">{row.point_count}</span>,
    },
    {
      title: t('inspection.status'),
      dataIndex: 'status',
      width: 100,
      search: false,
      render: (_node, row) => {
        const meta = INSPECTION_STATUS_META[row.status]
        return <Tag color={meta.color}>{meta.label}</Tag>
      },
    },
    {
      title: t('inspection.actions'),
      key: 'actions',
      width: 90,
      search: false,
      render: (_node, row) => (
        <Button type="link" size="small" onClick={() => navigate(`/inspections/${row.id}`)}>
          {t('inspection.detail')}
        </Button>
      ),
    },
  ]

  return (
    <div className="app-page">
      <ProTable<Inspection>
        rowKey="id"
        columns={columns}
        dataSource={rows}
        loading={isLoading}
        search={false}
        cardBordered
        headerTitle={t('inspection.title')}
        options={{ reload: false, density: false, setting: false }}
        pagination={{
          current: page,
          pageSize: PAGE_SIZE,
          total: data?.total ?? 0,
          showSizeChanger: false,
          onChange: (next) => setPage(next),
        }}
        toolbar={{
          actions: [
            <ExportButton
              key="export"
              type="INSPECTION_RECORDS"
              label={t('exports.inspectionRecords')}
              params={{
                project_id: projectId,
                inspector_id: inspectorId,
                status,
                from,
                to,
              }}
            />,
          ],
          filter: (
            <Space size="middle" wrap>
              <Select<number>
                allowClear
                placeholder={t('inspection.project')}
                style={{ minWidth: 160 }}
                value={projectId}
                onChange={(value) => {
                  setProjectId(value)
                  resetPage()
                }}
                options={projectOptions}
              />
              <Select<number>
                allowClear
                placeholder={t('inspection.inspector')}
                style={{ minWidth: 140 }}
                value={inspectorId}
                onChange={(value) => {
                  setInspectorId(value)
                  resetPage()
                }}
                options={inspectorOptions}
              />
              <DatePicker.RangePicker
                showTime
                value={range}
                onChange={(value) => {
                  setRange(value as [Dayjs, Dayjs] | null)
                  resetPage()
                }}
              />
              <Select<InspectionStatus>
                allowClear
                placeholder={t('inspection.status')}
                style={{ minWidth: 120 }}
                value={status}
                onChange={(value) => {
                  setStatus(value)
                  resetPage()
                }}
                options={INSPECTION_STATUS_ORDER.map((s) => ({
                  value: s,
                  label: INSPECTION_STATUS_META[s].label,
                }))}
              />
            </Space>
          ),
        }}
        locale={{ emptyText: <EmptyState description={t('inspection.empty')} /> }}
      />
    </div>
  )
}
