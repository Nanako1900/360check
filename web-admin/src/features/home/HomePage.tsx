import { Card, Typography } from 'antd'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/shared/auth/authStore'

const { Title, Paragraph } = Typography

/** 工作台首页（P0 占位仪表盘，后续 Phase 富化）。bento 网格突出关键入口（§10.4）。 */
export function HomePage() {
  const { t } = useTranslation()
  const user = useAuthStore((s) => s.user)
  const permissionCodes = useAuthStore((s) => s.permissionCodes)

  return (
    <div className="app-page">
      <Title level={3} style={{ marginTop: 0 }}>
        {t('menu.home')}
      </Title>
      <Paragraph type="secondary">
        {t('home.welcomeLead', { name: user?.display_name ?? user?.username ?? '' })}{' '}
        <span className="num">{permissionCodes.length}</span> {t('home.welcomeTail')}
      </Paragraph>
      <div
        style={{
          display: 'grid',
          gap: 'var(--space-5)',
          gridTemplateColumns: 'repeat(auto-fill, minmax(220px, 1fr))',
          marginTop: 'var(--space-6)',
        }}
      >
        <Card title={t('home.cardProjInspTitle')} variant="borderless">
          <Paragraph type="secondary">{t('home.cardProjInspDesc')}</Paragraph>
        </Card>
        <Card title={t('home.cardMapPanoTitle')} variant="borderless">
          <Paragraph type="secondary">{t('home.cardMapPanoDesc')}</Paragraph>
        </Card>
        <Card title={t('home.cardProblemTitle')} variant="borderless">
          <Paragraph type="secondary">{t('home.cardProblemDesc')}</Paragraph>
        </Card>
      </div>
    </div>
  )
}
