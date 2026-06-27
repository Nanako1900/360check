import { useMemo } from 'react'
import { Button, Card, Descriptions, Space, Spin, Tag, Typography } from 'antd'
import { useTranslation } from 'react-i18next'
import { useNavigate, useParams } from 'react-router-dom'
import type { DictItem } from '@/shared/api/types'
import { EmptyState } from '@/shared/ui/EmptyState'
import { fmtDate } from '@/shared/time/dayjs'
import { useDict } from '@/shared/dict/useDict'
import { useProject } from './api'
import { TaskTable } from './TaskTable'
import { ProjectInspectionsCard } from './ProjectInspectionsCard'
import { PROJECT_STATUS_META } from './status'

const { Title } = Typography

/** custom_fields 显示：以 project_field 字典项的 label 渲染（含已退役项，避免「未知」）。 */
function customFieldEntries(
  custom: Record<string, unknown> | undefined,
  fieldItems: DictItem[],
): Array<{ key: string; label: string; value: string }> {
  if (!custom) return []
  const labelByCode = new Map<string, string>()
  for (const i of fieldItems) labelByCode.set(i.code, i.label)
  return Object.entries(custom).map(([key, value]) => ({
    key,
    label: labelByCode.get(key) ?? key,
    value: value === null || value === undefined ? '—' : String(value),
  }))
}

export function ProjectDetailPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { id } = useParams<{ id: string }>()
  const projectId = Number(id)

  const { data: project, isLoading, isError } = useProject(projectId)
  const { data: fieldDict } = useDict('project_field')
  const cfEntries = useMemo(
    () => customFieldEntries(project?.custom_fields, fieldDict?.items ?? []),
    [project, fieldDict],
  )

  if (isLoading) {
    return (
      <div className="app-page" style={{ display: 'grid', placeItems: 'center', minHeight: 240 }}>
        <Spin />
      </div>
    )
  }
  if (isError || !project) {
    return (
      <div className="app-page">
        <EmptyState description={t('proj.notFound')} />
      </div>
    )
  }

  const statusMeta = PROJECT_STATUS_META[project.status]

  return (
    <div className="app-page">
      <Space style={{ marginBlockEnd: 'var(--space-5)' }}>
        <Button onClick={() => navigate('/projects')}>{t('common.back')}</Button>
        <Title level={3} style={{ margin: 0 }}>
          {project.name}
        </Title>
        <Tag color={statusMeta.color}>{statusMeta.label}</Tag>
      </Space>

      <Card bordered style={{ marginBlockEnd: 'var(--space-5)' }}>
        <Descriptions column={2} size="middle">
          <Descriptions.Item label={t('proj.code')}>
            <span className="tabular">{project.code}</span>
          </Descriptions.Item>
          <Descriptions.Item label={t('proj.status')}>
            <Tag color={statusMeta.color}>{statusMeta.label}</Tag>
          </Descriptions.Item>
          <Descriptions.Item label={t('proj.startDate')}>
            {project.start_date ? (
              <span className="tabular">{fmtDate(`${project.start_date}T00:00:00Z`)}</span>
            ) : (
              '—'
            )}
          </Descriptions.Item>
          <Descriptions.Item label={t('proj.endDate')}>
            {project.end_date ? (
              <span className="tabular">{fmtDate(`${project.end_date}T00:00:00Z`)}</span>
            ) : (
              '—'
            )}
          </Descriptions.Item>
          {project.description ? (
            <Descriptions.Item label={t('proj.description')} span={2}>
              {project.description}
            </Descriptions.Item>
          ) : null}
          {cfEntries.map((entry) => (
            <Descriptions.Item key={entry.key} label={entry.label}>
              {entry.value}
            </Descriptions.Item>
          ))}
        </Descriptions>
      </Card>

      <Card bordered title={t('proj.relatedTasks')} style={{ marginBlockEnd: 'var(--space-5)' }}>
        <TaskTable projectId={project.id} />
      </Card>

      <ProjectInspectionsCard projectId={project.id} />
    </div>
  )
}
