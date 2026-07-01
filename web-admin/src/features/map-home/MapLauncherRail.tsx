/**
 * MapLauncherRail（§P1）：悬浮左栏 —— 图层开关（项目范围 / 问题点）+ 快捷入口（按权限过滤的现有页面）
 * + 未定位项目列表（无 area_geom，点击去项目详情设置区域）。折叠时整体隐藏。
 */
import { Switch } from 'antd'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { MENU, isMenuItemVisible } from '@/routes/menu'
import type { Project } from '@/shared/api/types'

export interface MapLayers {
  areas: boolean
  problems: boolean
}

interface MapLauncherRailProps {
  collapsed: boolean
  layers: MapLayers
  onToggleLayer: (key: keyof MapLayers, value: boolean) => void
  /** 无地理范围的项目（不在地图上），列出以便去设置。 */
  unlocatedProjects: Project[]
  can: (code: string) => boolean
}

export function MapLauncherRail({
  collapsed,
  layers,
  onToggleLayer,
  unlocatedProjects,
  can,
}: MapLauncherRailProps) {
  const { t } = useTranslation()
  const links = MENU.filter((m) => m.path !== '/' && isMenuItemVisible(m, can))

  return (
    <div
      className={`map-home__rail map-home__glass${collapsed ? ' map-home__rail--collapsed' : ''}`}
      aria-hidden={collapsed}
    >
      <div>
        <div className="map-home__rail-section-title">{t('mapHome.layers')}</div>
        <div className="map-home__links">
          <div className="map-home__link">
            <Switch
              size="small"
              checked={layers.areas}
              onChange={(v) => onToggleLayer('areas', v)}
              aria-label={t('mapHome.layerAreas')}
            />
            <span>{t('mapHome.layerAreas')}</span>
          </div>
          <div className="map-home__link">
            <Switch
              size="small"
              checked={layers.problems}
              onChange={(v) => onToggleLayer('problems', v)}
              aria-label={t('mapHome.layerProblems')}
            />
            <span>{t('mapHome.layerProblems')}</span>
          </div>
        </div>
      </div>

      {links.length > 0 ? (
        <div>
          <div className="map-home__rail-section-title">{t('mapHome.quickEntry')}</div>
          <div className="map-home__links">
            {links.map((m) => (
              <Link key={m.path} className="map-home__link" to={m.path}>
                {m.icon}
                <span>{t(m.name)}</span>
              </Link>
            ))}
          </div>
        </div>
      ) : null}

      {unlocatedProjects.length > 0 ? (
        <div>
          <div className="map-home__rail-section-title">{t('mapHome.unlocated')}</div>
          <div className="map-home__links">
            {unlocatedProjects.map((p) => (
              <Link
                key={p.id}
                className="map-home__link map-home__unlocated"
                to={`/projects/${p.id}`}
              >
                {p.name}
              </Link>
            ))}
          </div>
        </div>
      ) : null}
    </div>
  )
}
