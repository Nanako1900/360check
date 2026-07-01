/**
 * MapSelectionCard（§P1）：点选项目后底部浮出的摘要卡 —— 名称/编号/状态 + 问题数，
 * 以及跳转「项目详情 / 巡查记录 / 问题列表」的入口（复用现有页面，落地方式=保留现有页）。
 */
import { Button, Card, Space, Tag, Typography } from 'antd'
import { CloseOutlined } from '@ant-design/icons'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import type { Project } from '@/shared/api/types'

const { Text } = Typography

interface MapSelectionCardProps {
  project: Project
  /** 已加载则显示问题数；null 表示未加载/未开启问题图层。 */
  problemCount: number | null
  onClose: () => void
}

export function MapSelectionCard({ project, problemCount, onClose }: MapSelectionCardProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()

  return (
    <Card
      className="map-home__card map-home__glass"
      size="small"
      variant="borderless"
      title={
        <Space size={8}>
          <span>{project.name}</span>
          <Tag>{project.status}</Tag>
        </Space>
      }
      extra={
        <Button
          type="text"
          size="small"
          icon={<CloseOutlined />}
          aria-label={t('common.cancel')}
          onClick={onClose}
        />
      }
    >
      <Space direction="vertical" size={8} style={{ width: '100%' }}>
        <Text type="secondary">
          {project.code}
          {problemCount != null ? ` · ${t('mapHome.problemCount', { count: problemCount })}` : ''}
        </Text>
        <Space wrap>
          <Button type="primary" size="small" onClick={() => navigate(`/projects/${project.id}`)}>
            {t('mapHome.projectDetail')}
          </Button>
          <Button size="small" onClick={() => navigate(`/inspections?project_id=${project.id}`)}>
            {t('mapHome.inspections')}
          </Button>
          <Button size="small" onClick={() => navigate(`/problems?project_id=${project.id}`)}>
            {t('mapHome.problems')}
          </Button>
        </Space>
      </Space>
    </Card>
  )
}
