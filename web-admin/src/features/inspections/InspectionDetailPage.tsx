import { Button, Card, Descriptions, Space, Spin, Tag, Typography } from 'antd'
import { EnvironmentOutlined, WarningOutlined } from '@ant-design/icons'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { EmptyState } from '@/shared/ui/EmptyState'
import { fmt } from '@/shared/time/dayjs'
import { formatDuration, formatMileageKm } from '@/shared/util/format'
import { useProject } from '@/features/projects/api'
import { useUsers } from '@/features/rbac/api'
import { useInspection } from './api'
import { INSPECTION_STATUS_META } from './status'

const { Title } = Typography

export function InspectionDetailPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { id } = useParams<{ id: string }>()
  const inspectionId = Number(id)

  const { data: inspection, isLoading, isError } = useInspection(inspectionId)
  const { data: project } = useProject(inspection?.project_id ?? 0, Boolean(inspection))
  const { data: usersData } = useUsers({ page: 1, page_size: 200 })
  const inspector = usersData?.items.find((u) => u.id === inspection?.inspector_id)

  if (isLoading) {
    return (
      <div className="app-page" style={{ display: 'grid', placeItems: 'center', minHeight: 240 }}>
        <Spin />
      </div>
    )
  }
  if (isError || !inspection) {
    return (
      <div className="app-page">
        <EmptyState description={t('inspection.notFound')} />
      </div>
    )
  }

  const inProgress = inspection.status === 'IN_PROGRESS'
  const statusMeta = INSPECTION_STATUS_META[inspection.status]

  return (
    <div className="app-page">
      <Space style={{ marginBlockEnd: 'var(--space-5)' }}>
        <Button onClick={() => navigate(-1)}>{t('common.back')}</Button>
        <Title level={3} style={{ margin: 0 }}>
          {t('inspection.detailTitle')}
        </Title>
        <Tag color={statusMeta.color}>{statusMeta.label}</Tag>
      </Space>

      <Card bordered style={{ marginBlockEnd: 'var(--space-5)' }}>
        <Descriptions column={2} size="middle">
          <Descriptions.Item label={t('inspection.project')}>
            {project ? (
              <Link to={`/projects/${project.id}`}>{project.name}</Link>
            ) : (
              `#${inspection.project_id}`
            )}
          </Descriptions.Item>
          <Descriptions.Item label={t('inspection.inspector')}>
            {inspector ? inspector.display_name || inspector.username : `#${inspection.inspector_id}`}
          </Descriptions.Item>
          <Descriptions.Item label={t('inspection.startedAt')}>
            <span className="tabular">{fmt(inspection.started_at)}</span>
          </Descriptions.Item>
          <Descriptions.Item label={t('inspection.endedAt')}>
            {inspection.ended_at ? (
              <span className="tabular">{fmt(inspection.ended_at)}</span>
            ) : (
              <span style={{ color: 'var(--color-ink-muted)' }}>{t('inspection.inProgress')}</span>
            )}
          </Descriptions.Item>
          <Descriptions.Item label={t('inspection.pointCount')}>
            <span className="tabular">{inspection.point_count}</span>
          </Descriptions.Item>
          {/* 进行中巡查 route_geom 为 NULL → 隐藏里程/时长，提示进行中（§P3 关键坑）。 */}
          {inProgress ? (
            <Descriptions.Item label={t('inspection.mileage')}>
              <span style={{ color: 'var(--color-ink-muted)' }}>{t('inspection.inProgress')}</span>
            </Descriptions.Item>
          ) : (
            <>
              <Descriptions.Item label={t('inspection.duration')}>
                <span className="tabular">{formatDuration(inspection.duration_seconds)}</span>
              </Descriptions.Item>
              <Descriptions.Item label={t('inspection.mileage')}>
                <span className="tabular">{formatMileageKm(inspection.mileage_meters)}</span>
              </Descriptions.Item>
            </>
          )}
          {inspection.note ? (
            <Descriptions.Item label={t('inspection.note')} span={2}>
              {inspection.note}
            </Descriptions.Item>
          ) : null}
        </Descriptions>
      </Card>

      <Space wrap>
        <Button
          type="primary"
          icon={<EnvironmentOutlined />}
          disabled={inProgress}
          onClick={() => navigate(`/inspections/${inspection.id}/map`)}
        >
          {t('inspection.viewMap')}
        </Button>
        <Button
          icon={<WarningOutlined />}
          onClick={() => navigate(`/problems?inspection_id=${inspection.id}`)}
        >
          {t('inspection.viewProblems')}
        </Button>
      </Space>
    </div>
  )
}
