import { useMemo, useState } from 'react'
import { Button, DatePicker, Select, Space } from 'antd'
import { ProTable } from '@ant-design/pro-components'
import type { ProColumns } from '@ant-design/pro-components'
import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router-dom'
import type { Dayjs } from 'dayjs'
import type { Problem, Project, User } from '@/shared/api/types'
import { DictTag } from '@/shared/ui/DictTag'
import { EmptyState } from '@/shared/ui/EmptyState'
import { fmt, toUtcIso } from '@/shared/time/dayjs'
import { useUsers } from '@/features/rbac/api'
import { useProjects } from '@/features/projects/api'
import { ExportButton } from '@/features/exports/ExportButton'
import { useProblems } from './api'
import { useActiveDictOptions } from './dictOptions'
import { ProblemCover } from './ProblemCover'
import { ProblemDetailDrawer } from './ProblemDetailDrawer'

const PAGE_SIZE = 10

export function ProblemListPage() {
  const { t } = useTranslation()
  const [searchParams] = useSearchParams()
  const initialInspectionId = searchParams.get('inspection_id')

  const [page, setPage] = useState(1)
  const [projectId, setProjectId] = useState<number | undefined>(undefined)
  const [typeId, setTypeId] = useState<number | undefined>(undefined)
  const [statusId, setStatusId] = useState<number | undefined>(undefined)
  const [categoryId, setCategoryId] = useState<number | undefined>(undefined)
  const [inspectorId, setInspectorId] = useState<number | undefined>(undefined)
  const [range, setRange] = useState<[Dayjs, Dayjs] | null>(null)
  // D1：巡查详情「该巡查的问题」入口经 ?inspection_id= 进入；仅作过滤，不再提供下拉。
  const inspectionId = initialInspectionId ? Number(initialInspectionId) : undefined

  const [detailId, setDetailId] = useState<number | null>(null)

  const from = range?.[0] ? toUtcIso(range[0]) : undefined
  const to = range?.[1] ? toUtcIso(range[1]) : undefined

  const { data, isLoading } = useProblems({
    project_id: projectId,
    type: typeId,
    status: statusId,
    category: categoryId,
    inspector_id: inspectorId,
    inspection_id: inspectionId,
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

  // 筛选下拉只列 active 字典项；表格渲染用 DictTag（含退役项历史容忍）。
  const typeOptions = useActiveDictOptions('problem_type')
  const statusOptions = useActiveDictOptions('problem_status')
  const categoryOptions = useActiveDictOptions('problem_category')

  const resetPage = (): void => setPage(1)

  const columns: ProColumns<Problem>[] = [
    {
      title: t('problem.cover'),
      dataIndex: 'cover_media_id',
      width: 80,
      search: false,
      render: (_node, row) => <ProblemCover mediaId={row.cover_media_id ?? null} />,
    },
    {
      title: t('problem.type'),
      dataIndex: 'type_item_id',
      width: 110,
      search: false,
      render: (_node, row) => <DictTag code="problem_type" itemId={row.type_item_id} />,
    },
    {
      title: t('problem.status'),
      dataIndex: 'status_item_id',
      width: 120,
      search: false,
      render: (_node, row) => <DictTag code="problem_status" itemId={row.status_item_id} />,
    },
    {
      title: t('problem.category'),
      dataIndex: 'category_item_id',
      width: 110,
      search: false,
      render: (_node, row) => <DictTag code="problem_category" itemId={row.category_item_id} />,
    },
    {
      title: t('problem.titleCol'),
      dataIndex: 'title',
      ellipsis: true,
      search: false,
      render: (_node, row) =>
        row.title || <span style={{ color: 'var(--color-ink-muted)' }}>—</span>,
    },
    {
      title: t('problem.project'),
      dataIndex: 'project_id',
      width: 150,
      search: false,
      render: (_node, row) => projectById.get(row.project_id)?.name ?? `#${row.project_id}`,
    },
    {
      title: t('problem.inspector'),
      dataIndex: 'inspector_id',
      width: 110,
      search: false,
      render: (_node, row) => {
        const u = userById.get(row.inspector_id)
        return u ? u.display_name || u.username : `#${row.inspector_id}`
      },
    },
    {
      title: t('problem.capturedAt'),
      dataIndex: 'captured_at',
      width: 150,
      search: false,
      render: (_node, row) => <span className="tabular">{fmt(row.captured_at)}</span>,
    },
    {
      title: t('problem.actions'),
      key: 'actions',
      width: 90,
      search: false,
      render: (_node, row) => (
        <Button type="link" size="small" onClick={() => setDetailId(row.id)}>
          {t('problem.detail')}
        </Button>
      ),
    },
  ]

  return (
    <div className="app-page">
      <ProTable<Problem>
        rowKey="id"
        columns={columns}
        dataSource={rows}
        loading={isLoading}
        search={false}
        cardBordered
        headerTitle={t('problem.title')}
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
              type="PROBLEM_LIST"
              label={t('exports.problemList')}
              params={{
                project_id: projectId,
                type: typeId,
                status: statusId,
                category: categoryId,
                inspector_id: inspectorId,
                inspection_id: inspectionId,
                from,
                to,
              }}
            />,
          ],
          filter: (
            <Space size="middle" wrap>
              <Select<number>
                allowClear
                placeholder={t('problem.project')}
                style={{ minWidth: 150 }}
                value={projectId}
                onChange={(value) => {
                  setProjectId(value)
                  resetPage()
                }}
                options={projectOptions}
              />
              <Select<number>
                allowClear
                placeholder={t('problem.type')}
                style={{ minWidth: 120 }}
                value={typeId}
                onChange={(value) => {
                  setTypeId(value)
                  resetPage()
                }}
                options={typeOptions}
              />
              <Select<number>
                allowClear
                placeholder={t('problem.status')}
                style={{ minWidth: 120 }}
                value={statusId}
                onChange={(value) => {
                  setStatusId(value)
                  resetPage()
                }}
                options={statusOptions}
              />
              <Select<number>
                allowClear
                placeholder={t('problem.category')}
                style={{ minWidth: 120 }}
                value={categoryId}
                onChange={(value) => {
                  setCategoryId(value)
                  resetPage()
                }}
                options={categoryOptions}
              />
              <Select<number>
                allowClear
                placeholder={t('problem.inspector')}
                style={{ minWidth: 130 }}
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
            </Space>
          ),
        }}
        locale={{ emptyText: <EmptyState description={t('problem.empty')} /> }}
      />
      <ProblemDetailDrawer problemId={detailId} onClose={() => setDetailId(null)} />
    </div>
  )
}
