import { Card, Typography } from 'antd'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/shared/auth/authStore'
import { MENU, isMenuItemVisible } from '@/routes/menu'
import { useStatsOverview } from '@/features/stats/api'
import { formatDuration, formatMileageKm } from '@/shared/util/format'

const { Title, Paragraph, Text } = Typography

/**
 * 工作台首页：
 * - 概览指标：无筛选的全局 /stats/overview（仅 stats:read 拉取，避免 403）。
 * - 快捷入口：复用菜单配置，按当前账号权限过滤，直达各功能页（替代旧的「将在 PX 上线」占位）。
 */
export function HomePage() {
  const { t } = useTranslation()
  const user = useAuthStore((s) => s.user)
  const permissionCodes = useAuthStore((s) => s.permissionCodes)
  const can = (code: string) => permissionCodes.includes(code)

  const canStats = can('stats:read')
  const { data: stats } = useStatsOverview({}, canStats)

  // 功能入口卡片：排除「工作台」自身，按权限过滤（与左侧菜单同源）。
  const entries = MENU.filter((m) => m.path !== '/' && isMenuItemVisible(m, can))

  const metrics = [
    { label: t('stats.inspectionCount'), value: String(stats?.inspection_count ?? 0) },
    { label: t('stats.problemCount'), value: String(stats?.problem_count ?? 0) },
    { label: t('stats.totalMileage'), value: formatMileageKm(stats?.total_mileage_meters ?? 0) },
    { label: t('stats.avgDuration'), value: formatDuration(stats?.avg_duration_seconds ?? 0) },
    { label: t('stats.totalDuration'), value: formatDuration(stats?.total_duration_seconds ?? 0) },
  ]

  return (
    <div className="app-page">
      <Title level={3} style={{ marginTop: 0 }}>
        {t('menu.home')}
      </Title>
      <Paragraph type="secondary">
        {t('home.welcomeLead', { name: user?.display_name ?? user?.username ?? '' })}{' '}
        <span className="num">{permissionCodes.length}</span> {t('home.welcomeTail')}
      </Paragraph>

      {canStats && stats ? (
        <>
          <Title level={5} style={{ marginTop: 'var(--space-6)' }}>
            {t('home.overview')}
          </Title>
          <div
            style={{
              display: 'grid',
              gap: 'var(--space-5)',
              gridTemplateColumns: 'repeat(auto-fill, minmax(160px, 1fr))',
              marginTop: 'var(--space-4)',
            }}
          >
            {metrics.map((m) => (
              <Card key={m.label} variant="borderless">
                <Text type="secondary" style={{ display: 'block', fontSize: 13 }}>
                  {m.label}
                </Text>
                <Text strong style={{ fontSize: 24, fontVariantNumeric: 'tabular-nums' }}>
                  {m.value}
                </Text>
              </Card>
            ))}
          </div>
        </>
      ) : null}

      {entries.length > 0 ? (
        <>
          <Title level={5} style={{ marginTop: 'var(--space-7)' }}>
            {t('home.quickEntry')}
          </Title>
          <div
            style={{
              display: 'grid',
              gap: 'var(--space-5)',
              gridTemplateColumns: 'repeat(auto-fill, minmax(220px, 1fr))',
              marginTop: 'var(--space-4)',
            }}
          >
            {entries.map((m) => (
              <Link key={m.path} to={m.path}>
                <Card
                  hoverable
                  variant="borderless"
                  title={
                    <span>
                      {m.icon} {t(m.name)}
                    </span>
                  }
                >
                  <Paragraph type="secondary" style={{ margin: 0 }}>
                    {t(`home.entry.${m.path.slice(1)}`)}
                  </Paragraph>
                </Card>
              </Link>
            ))}
          </div>
        </>
      ) : null}
    </div>
  )
}
