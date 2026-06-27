import { Button, Result } from 'antd'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'

/** 403 页：无权限访问受限路由（守卫命中）。前端权限仅 UI 层（§4.6 FORBIDDEN）。 */
export function Forbidden() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  return (
    <Result
      status="403"
      title="403"
      subTitle={t('error.forbiddenSubtitle')}
      extra={
        <Button type="primary" onClick={() => navigate('/')}>
          {t('menu.home')}
        </Button>
      }
    />
  )
}
