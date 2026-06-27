import { Tabs, Typography } from 'antd'
import { useTranslation } from 'react-i18next'
import { DictTypeListPage } from './DictTypeListPage'
import { ConfigPage } from './ConfigPage'

const { Title } = Typography

/** 字典/配置管理容器（路由 /dict 目标）：字典类型 / 系统配置 两个标签页。 */
export function DictConfigPage() {
  const { t } = useTranslation()
  return (
    <div className="app-page">
      <Title level={3} style={{ marginTop: 0 }}>
        {t('dict.title')}
      </Title>
      <Tabs
        defaultActiveKey="types"
        items={[
          { key: 'types', label: t('dict.tabTypes'), children: <DictTypeListPage /> },
          { key: 'config', label: t('dict.tabConfig'), children: <ConfigPage /> },
        ]}
      />
    </div>
  )
}
