/**
 * MapHome（§P1 地图主页）：全屏地图 + 悬浮菜单（导航 App 风）。
 *
 * 首屏「所有项目总览」：拉 /projects，把带 area_geom 的项目画成范围+质心标记（ProjectAreaLayer）；
 * 点项目 → 飞行定位 + 加载该项目问题点（ProblemMarkerLayer）→ 点问题看全景/详情。
 * 无 VITE_MAP_KEY / 无带范围项目 / 加载中 均有全屏降级浮层（§10.2）。
 * 落地方式=保留现有页面：左栏快捷入口跳转现有管理页；旧工作台仪表盘在 /dashboard。
 */
import { useEffect, useMemo, useRef, useState } from 'react'
import { Button, Space, Spin, Typography } from 'antd'
import { AimOutlined, MinusOutlined, PlusOutlined } from '@ant-design/icons'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { env } from '@/env'
import { EmptyState } from '@/shared/ui/EmptyState'
import { useAuthStore } from '@/shared/auth/authStore'
import { useLogout } from '@/shared/auth/useLogout'
import { ChangePasswordModal } from '@/features/login/ChangePasswordModal'
import { useTencentMap } from '@/shared/map/useTencentMap'
import { ProjectAreaLayer } from '@/shared/map/ProjectAreaLayer'
import { ProblemMarkerLayer } from '@/shared/map/ProblemMarkerLayer'
import { MapInfoWindow } from '@/shared/map/MapInfoWindow'
import type { InfoWindowLabels, InfoWindowTarget } from '@/shared/map/MapInfoWindow'
import { fitBoundsToPoints } from '@/shared/map/fitBounds'
import { centroid, hasArea, projectPoints } from '@/shared/map/projectGeo'
import { wgsToTMapLatLng } from '@/shared/map/toLatLng'
import type { ProblemFeatureProperties } from '@/shared/api/types'
import { useProjects } from '@/features/projects/api'
import { useProblemsMap } from '@/features/problems/api'
import { PanoramaModal } from '@/features/panorama/PanoramaModal'
import type { PanoramaContext } from '@/features/panorama/PanoramaModal'
import { MapTopBar } from './MapTopBar'
import { MapLauncherRail } from './MapLauncherRail'
import type { MapLayers } from './MapLauncherRail'
import { MapSelectionCard } from './MapSelectionCard'
import './map-home.css'

const { Text } = Typography

export function MapHome() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const user = useAuthStore((s) => s.user)
  const permissionCodes = useAuthStore((s) => s.permissionCodes)
  const can = (code: string): boolean => permissionCodes.includes(code)
  const logout = useLogout()

  const [railCollapsed, setRailCollapsed] = useState(false)
  const [layers, setLayers] = useState<MapLayers>({ areas: true, problems: true })
  const [selectedProjectId, setSelectedProjectId] = useState<number | null>(null)
  const [selectedPoint, setSelectedPoint] = useState<InfoWindowTarget | null>(null)
  const [panorama, setPanorama] = useState<{ mediaId: number; context: PanoramaContext } | null>(
    null,
  )
  const [pwOpen, setPwOpen] = useState(false)
  const zoomRef = useRef(14)

  const { containerRef, map, TMap, status, error } = useTencentMap({ apiKey: env.VITE_MAP_KEY })
  const { data: projectsData } = useProjects({ page: 1, page_size: 200 })

  const projects = useMemo(() => projectsData?.items ?? [], [projectsData])
  const withArea = useMemo(() => projects.filter(hasArea), [projects])
  const withoutArea = useMemo(() => projects.filter((p) => !hasArea(p)), [projects])
  const selectedProject = useMemo(
    () => projects.find((p) => p.id === selectedProjectId) ?? null,
    [projects, selectedProjectId],
  )

  const canViewProjects = can('project:read')
  const problemsEnabled = selectedProjectId != null && layers.problems && can('problem:read')
  const { data: problems } = useProblemsMap(
    { project_id: selectedProjectId ?? undefined },
    problemsEnabled,
  )

  const infoLabels: InfoWindowLabels = useMemo(
    () => ({
      type: t('map.infoType'),
      status: t('map.infoStatus'),
      viewPanorama: t('map.viewPanorama'),
      viewDetail: t('map.viewDetail'),
      noThumb: t('map.noThumb'),
      untitled: t('map.untitled'),
    }),
    [t],
  )

  // 选中项目 → 飞行定位到其质心。
  useEffect(() => {
    if (!map || !TMap || !selectedProject) return
    const c = centroid(projectPoints(selectedProject))
    if (c) {
      map.setCenter(wgsToTMapLatLng(c, TMap))
      zoomRef.current = 15
      map.setZoom(15)
    }
  }, [map, TMap, selectedProject])

  function handleSelectProject(id: number): void {
    setSelectedPoint(null)
    setSelectedProjectId(id)
  }
  function handleLogout(): void {
    logout.mutate(undefined, { onSettled: () => navigate('/login', { replace: true }) })
  }
  function handleZoom(delta: number): void {
    if (!map) return
    zoomRef.current = Math.max(3, Math.min(20, zoomRef.current + delta))
    map.setZoom(zoomRef.current)
  }
  function handleFitAll(): void {
    if (map && TMap) fitBoundsToPoints(map, TMap, withArea.flatMap(projectPoints))
  }
  function handleViewPanorama(props: ProblemFeatureProperties): void {
    if (props.cover_media_id == null) {
      navigate(`/panorama?problem_id=${props.id}`)
      return
    }
    setPanorama({
      mediaId: props.cover_media_id,
      context: {
        inspectionStartedAt: null,
        problemTypeItemId: props.type_item_id ?? null,
        problemStatusItemId: props.status_item_id ?? null,
        problemTitle: props.title ?? null,
      },
    })
  }
  function handleViewDetail(props: ProblemFeatureProperties): void {
    navigate(`/problems?problem_id=${props.id}`)
  }

  const noProjectsWithArea = status === 'ready' && canViewProjects && withArea.length === 0

  return (
    <div className="map-home">
      <div
        ref={containerRef}
        className="map-home__canvas"
        role="region"
        aria-label={t('menu.home')}
      />

      {status === 'loading' ? (
        <div className="map-home__overlay" aria-live="polite">
          <Space direction="vertical" align="center" size={8}>
            <Spin />
            <Text type="secondary">{t('map.loading')}</Text>
          </Space>
        </div>
      ) : null}

      {status === 'error' ? (
        <div className="map-home__overlay">
          <div className="map-home__overlay-box">
            <EmptyState description={error ?? t('map.loadFailedHint')} />
            <Text type="secondary">{t('mapHome.noKeyHint')}</Text>
          </div>
        </div>
      ) : null}

      {noProjectsWithArea ? (
        <div className="map-home__overlay">
          <div className="map-home__overlay-box">
            <EmptyState description={t('mapHome.noAreaHint')} />
          </div>
        </div>
      ) : null}

      <MapTopBar
        title={t('menu.home')}
        userName={user?.display_name ?? user?.username ?? ''}
        projects={withArea}
        onPickProject={handleSelectProject}
        onToggleRail={() => setRailCollapsed((v) => !v)}
        onChangePassword={() => setPwOpen(true)}
        onLogout={handleLogout}
      />

      <MapLauncherRail
        collapsed={railCollapsed}
        layers={layers}
        onToggleLayer={(key, value) => setLayers((prev) => ({ ...prev, [key]: value }))}
        unlocatedProjects={withoutArea}
        can={can}
      />

      <div className="map-home__controls">
        <Button
          className="map-home__glass"
          shape="circle"
          icon={<PlusOutlined />}
          aria-label={t('mapHome.zoomIn')}
          onClick={() => handleZoom(1)}
        />
        <Button
          className="map-home__glass"
          shape="circle"
          icon={<MinusOutlined />}
          aria-label={t('mapHome.zoomOut')}
          onClick={() => handleZoom(-1)}
        />
        <Button
          className="map-home__glass"
          shape="circle"
          icon={<AimOutlined />}
          aria-label={t('mapHome.fitAll')}
          onClick={handleFitAll}
        />
      </div>

      {status === 'ready' && map && TMap ? (
        <>
          {layers.areas && canViewProjects ? (
            <ProjectAreaLayer
              map={map}
              TMap={TMap}
              projects={withArea}
              onSelect={handleSelectProject}
            />
          ) : null}
          {problemsEnabled ? (
            <>
              <ProblemMarkerLayer map={map} TMap={TMap} data={problems} onSelect={setSelectedPoint} />
              <MapInfoWindow
                map={map}
                TMap={TMap}
                target={selectedPoint}
                labels={infoLabels}
                onViewPanorama={handleViewPanorama}
                onViewDetail={handleViewDetail}
                onClose={() => setSelectedPoint(null)}
              />
            </>
          ) : null}
        </>
      ) : null}

      {selectedProject ? (
        <MapSelectionCard
          project={selectedProject}
          problemCount={problemsEnabled ? (problems?.features.length ?? null) : null}
          onClose={() => {
            setSelectedProjectId(null)
            setSelectedPoint(null)
          }}
        />
      ) : null}

      <PanoramaModal
        open={panorama !== null}
        mediaId={panorama?.mediaId ?? null}
        context={panorama?.context}
        onClose={() => setPanorama(null)}
      />
      <ChangePasswordModal open={pwOpen} onClose={() => setPwOpen(false)} onSuccess={handleLogout} />
    </div>
  )
}
