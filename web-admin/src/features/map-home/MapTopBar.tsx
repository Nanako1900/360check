/**
 * MapTopBar（§P1）：悬浮顶栏 —— 折叠侧栏按钮 + 标题 + 项目搜索（选中即飞行定位）+ 用户菜单（改密/登出）。
 * 搜索用受控 `value=null` 的 Select 当「跳转」控件：选择后立即回调 onPickProject 并复位。
 */
import { Avatar, Button, Dropdown, Select, Space } from 'antd'
import { KeyOutlined, LogoutOutlined, MenuOutlined, UserOutlined } from '@ant-design/icons'
import { useTranslation } from 'react-i18next'
import type { Project } from '@/shared/api/types'

interface MapTopBarProps {
  title: string
  userName: string
  /** 可搜索的项目（一般为带地理范围的项目）。 */
  projects: Project[]
  onPickProject: (id: number) => void
  onToggleRail: () => void
  onChangePassword: () => void
  onLogout: () => void
}

export function MapTopBar({
  title,
  userName,
  projects,
  onPickProject,
  onToggleRail,
  onChangePassword,
  onLogout,
}: MapTopBarProps) {
  const { t } = useTranslation()
  const options = projects.map((p) => ({ value: p.id, label: p.name }))

  return (
    <div className="map-home__topbar map-home__glass">
      <Button
        type="text"
        icon={<MenuOutlined />}
        aria-label={t('mapHome.toggleRail')}
        onClick={onToggleRail}
      />
      <span className="map-home__title">{title}</span>
      <div className="map-home__topbar-spacer" />
      <Select<number>
        className="map-home__search"
        showSearch
        allowClear
        placeholder={t('mapHome.searchPlaceholder')}
        options={options}
        optionFilterProp="label"
        value={null}
        onChange={(v) => {
          if (typeof v === 'number') onPickProject(v)
        }}
      />
      <Dropdown
        menu={{
          items: [
            { key: 'password', icon: <KeyOutlined />, label: t('common.changePassword') },
            { type: 'divider' },
            { key: 'logout', icon: <LogoutOutlined />, label: t('common.logout'), danger: true },
          ],
          onClick: ({ key }) => {
            if (key === 'password') onChangePassword()
            if (key === 'logout') onLogout()
          },
        }}
      >
        <Space style={{ cursor: 'pointer' }}>
          <Avatar size="small" icon={<UserOutlined />} />
          <span>{userName}</span>
        </Space>
      </Dropdown>
    </div>
  )
}
