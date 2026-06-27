import { Button, Result } from 'antd'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'

/** 404 页。 */
export function NotFound() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  return (
    <Result
      status="404"
      title="404"
      subTitle={t('error.notFoundSubtitle')}
      extra={
        <Button type="primary" onClick={() => navigate('/')}>
          {t('menu.home')}
        </Button>
      }
    />
  )
}
