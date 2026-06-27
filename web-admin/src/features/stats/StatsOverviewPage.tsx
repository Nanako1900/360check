/**
 * StatsOverviewPage（§P7 / WEB-7）：数据统计看板。
 *
 * 筛选区：项目下拉 + 巡查员下拉 + 时间范围 RangePicker（本周/本月/自定义快捷预设）。时间本地
 * Asia/Shanghai → toUtcIso 传 from/to（§P3 时间约定）。指标卡走 bento 网格（§10.4），数字等宽对齐。
 * 图表（问题类型/状态/人员/项目）由后端聚合（D2），前端仅渲染；颜色取 dict_item.color（含退役项）。
 *
 * 聚合在后端：本页不做大数据聚合、不拉全量行（§P7 强约束）。导出按钮为 P8 占位（disabled）。
 */
import { useMemo, useState } from 'react'
import { Card, DatePicker, Select, Space, Typography } from 'antd'
import { BarChartOutlined } from '@ant-design/icons'
import { useTranslation } from 'react-i18next'
import type { Dayjs } from 'dayjs'
import { dayjs, toUtcIso } from '@/shared/time/dayjs'
import { formatDuration, formatMileageKm } from '@/shared/util/format'
import { EmptyState } from '@/shared/ui/EmptyState'
import { EChart } from '@/shared/ui/charts/EChart'
import { barOption, pieOption } from '@/shared/ui/charts/options'
import { useProjects } from '@/features/projects/api'
import { useUsers } from '@/features/rbac/api'
import { ExportButton } from '@/features/exports/ExportButton'
import type { CountBucket } from '@/shared/api/types'
import { useStatsOverview } from './api'
import './stats.css'

const { Title, Paragraph } = Typography

type RangePreset = 'week' | 'month' | 'custom'

/** 预设时间范围 → [from, to] 本地 Dayjs（本周/本月）。custom 由用户自选。 */
function presetRange(preset: Exclude<RangePreset, 'custom'>): [Dayjs, Dayjs] {
  const unit = preset === 'week' ? 'week' : 'month'
  return [dayjs().startOf(unit), dayjs().endOf(unit)]
}

interface MetricCardProps {
  label: string
  value: string
}

/** 指标卡：标签 + 等宽数字（§10.4 tabular-nums）。 */
function MetricCard({ label, value }: MetricCardProps) {
  return (
    <div className="stats-metric">
      <span className="stats-metric-label">{label}</span>
      <span className="stats-metric-value tabular">{value}</span>
    </div>
  )
}

export function StatsOverviewPage() {
  const { t } = useTranslation()

  const [projectId, setProjectId] = useState<number | undefined>(undefined)
  const [inspectorId, setInspectorId] = useState<number | undefined>(undefined)
  const [preset, setPreset] = useState<RangePreset>('custom')
  const [range, setRange] = useState<[Dayjs, Dayjs] | null>(null)

  const from = range?.[0] ? toUtcIso(range[0]) : undefined
  const to = range?.[1] ? toUtcIso(range[1]) : undefined

  const { data, isLoading, isError } = useStatsOverview({
    project_id: projectId,
    inspector_id: inspectorId,
    from,
    to,
  })

  const { data: projectsData } = useProjects({ page: 1, page_size: 200 })
  const projectOptions = (projectsData?.items ?? []).map((p) => ({ value: p.id, label: p.name }))

  const { data: usersData } = useUsers({ page: 1, page_size: 200 })
  const inspectorOptions = (usersData?.items ?? []).map((u) => ({
    value: u.id,
    label: u.display_name || u.username,
  }))

  const applyPreset = (next: RangePreset): void => {
    setPreset(next)
    if (next === 'custom') {
      setRange(null)
    } else {
      setRange(presetRange(next))
    }
  }

  const onRangeChange = (value: [Dayjs, Dayjs] | null): void => {
    setPreset('custom')
    setRange(value)
  }

  // 是否「全空」：四类计数桶均为空且核心指标为 0 → 展示统一空态。
  const hasData = useMemo(() => {
    if (!data) return false
    const buckets: CountBucket[][] = [
      data.counts_by_type,
      data.counts_by_status,
      data.counts_by_inspector,
      data.counts_by_project,
    ]
    const anyBucket = buckets.some((b) => b.length > 0)
    return anyBucket || data.inspection_count > 0 || data.problem_count > 0
  }, [data])

  return (
    <div className="app-page">
      <Title level={3} style={{ marginBlockEnd: 'var(--space-2)' }}>
        <BarChartOutlined style={{ marginInlineEnd: 'var(--space-3)' }} />
        {t('stats.title')}
      </Title>
      <Paragraph type="secondary">{t('stats.subtitle')}</Paragraph>

      <Card className="stats-filter-card" styles={{ body: { padding: 'var(--space-5)' } }}>
        <Space size="middle" wrap>
          <Select<number>
            allowClear
            placeholder={t('stats.project')}
            style={{ minWidth: 180 }}
            value={projectId}
            onChange={setProjectId}
            options={projectOptions}
          />
          <Select<number>
            allowClear
            placeholder={t('stats.inspector')}
            style={{ minWidth: 160 }}
            value={inspectorId}
            onChange={setInspectorId}
            options={inspectorOptions}
          />
          <Select<RangePreset>
            style={{ minWidth: 120 }}
            value={preset}
            onChange={applyPreset}
            options={[
              { value: 'week', label: t('stats.presetThisWeek') },
              { value: 'month', label: t('stats.presetThisMonth') },
              { value: 'custom', label: t('stats.presetCustom') },
            ]}
          />
          <DatePicker.RangePicker
            showTime
            value={range}
            onChange={(value) => onRangeChange(value as [Dayjs, Dayjs] | null)}
          />
          {/* 导出项目统计数据 Excel（PROJECT_STATS，复用本页筛选 = 所见即所导）。 */}
          <ExportButton
            type="PROJECT_STATS"
            label={t('stats.exportStats')}
            params={{
              project_id: projectId,
              inspector_id: inspectorId,
              from,
              to,
            }}
          />
        </Space>
      </Card>

      {isError ? (
        <Card style={{ marginBlockStart: 'var(--space-5)' }}>
          <EmptyState description={t('stats.loadFailedHint')} />
        </Card>
      ) : !isLoading && data && !hasData ? (
        <Card style={{ marginBlockStart: 'var(--space-5)' }}>
          <EmptyState description={t('stats.empty')} />
        </Card>
      ) : (
        <>
          {/* 指标卡：bento 网格（§10.4），数字等宽。 */}
          <div className="stats-metric-grid">
            <MetricCard
              label={t('stats.inspectionCount')}
              value={String(data?.inspection_count ?? 0)}
            />
            <MetricCard label={t('stats.problemCount')} value={String(data?.problem_count ?? 0)} />
            <MetricCard
              label={t('stats.totalMileage')}
              value={formatMileageKm(data?.total_mileage_meters ?? 0)}
            />
            <MetricCard
              label={t('stats.avgDuration')}
              value={formatDuration(data?.avg_duration_seconds ?? 0)}
            />
            <MetricCard
              label={t('stats.totalDuration')}
              value={formatDuration(data?.total_duration_seconds ?? 0)}
            />
          </div>

          {/* 图表网格：类型/状态饼图 + 人员/项目柱状。 */}
          <div className="stats-chart-grid">
            <Card className="stats-chart-card" title={t('stats.byType')}>
              {data && data.counts_by_type.length > 0 ? (
                <EChart option={pieOption(data.counts_by_type, '')} ariaLabel={t('stats.byType')} />
              ) : (
                <EmptyState description={t('stats.chartEmpty')} />
              )}
            </Card>
            <Card className="stats-chart-card" title={t('stats.byStatus')}>
              {data && data.counts_by_status.length > 0 ? (
                <EChart
                  option={pieOption(data.counts_by_status, '')}
                  ariaLabel={t('stats.byStatus')}
                />
              ) : (
                <EmptyState description={t('stats.chartEmpty')} />
              )}
            </Card>
            <Card className="stats-chart-card" title={t('stats.byInspector')}>
              {data && data.counts_by_inspector.length > 0 ? (
                <EChart
                  option={barOption(data.counts_by_inspector, '')}
                  ariaLabel={t('stats.byInspector')}
                />
              ) : (
                <EmptyState description={t('stats.chartEmpty')} />
              )}
            </Card>
            <Card className="stats-chart-card" title={t('stats.byProject')}>
              {data && data.counts_by_project.length > 0 ? (
                <EChart
                  option={barOption(data.counts_by_project, '')}
                  ariaLabel={t('stats.byProject')}
                />
              ) : (
                <EmptyState description={t('stats.chartEmpty')} />
              )}
            </Card>
          </div>
        </>
      )}
    </div>
  )
}
