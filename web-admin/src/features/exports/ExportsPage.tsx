import { Card, Space, Typography } from 'antd'
import {
  BarChartOutlined,
  FileSearchOutlined,
  ProfileOutlined,
  RightOutlined,
} from '@ant-design/icons'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'

const { Title, Paragraph, Text } = Typography

interface ExportSurface {
  to: string
  icon: React.ReactNode
  titleKey: string
  descKey: string
}

const SURFACES: ExportSurface[] = [
  {
    to: '/inspections',
    icon: <ProfileOutlined />,
    titleKey: 'exports.inspectionRecords',
    descKey: 'exports.hubInspectionDesc',
  },
  {
    to: '/problems',
    icon: <FileSearchOutlined />,
    titleKey: 'exports.problemList',
    descKey: 'exports.hubProblemDesc',
  },
  {
    to: '/stats',
    icon: <BarChartOutlined />,
    titleKey: 'exports.projectStats',
    descKey: 'exports.hubStatsDesc',
  },
]

/**
 * 数据导出入口聚合页（§P8）。
 *
 * 三种导出按「所见即所导」绑定到各自的筛选页面（巡查 / 问题 / 统计），导出按钮就在对应列表/看板上。
 * 本页提供清晰跳转入口，避免另起一套脱离筛选上下文的导出表单。
 */
export function ExportsPage() {
  const { t } = useTranslation()
  return (
    <div className="app-page">
      <Title level={3} style={{ marginBlockEnd: 'var(--space-2)' }}>
        {t('menu.exports')}
      </Title>
      <Paragraph type="secondary">{t('exports.hubSubtitle')}</Paragraph>

      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        {SURFACES.map((surface) => (
          <Link key={surface.to} to={surface.to} style={{ display: 'block' }}>
            <Card hoverable styles={{ body: { padding: 'var(--space-5)' } }}>
              <Space align="center" style={{ width: '100%', justifyContent: 'space-between' }}>
                <Space align="center" size="middle">
                  <span style={{ fontSize: 22, color: 'var(--color-accent, #1677ff)' }}>
                    {surface.icon}
                  </span>
                  <Space direction="vertical" size={0}>
                    <Text strong>{t(surface.titleKey)}</Text>
                    <Text type="secondary">{t(surface.descKey)}</Text>
                  </Space>
                </Space>
                <RightOutlined aria-hidden />
              </Space>
            </Card>
          </Link>
        ))}
      </Space>
    </div>
  )
}
