/**
 * InspectionMapPage（§P4）：单次巡查的轨迹折线 + 该巡查问题点 + 点击详情。
 *
 * 组合：useTencentMap（SDK + 实例）→ TrajectoryLayer（route）+ ProblemMarkerLayer（/problems/map?inspection_id=）
 * + MapInfoWindow（点击单点）。地图容器固定高度避免 CLS（§10.1）；SDK 加载失败优雅降级（§10.2）。
 */
import { useMemo, useState } from 'react'
import { Alert, Button, Space, Spin, Typography } from 'antd'
import { EnvironmentOutlined } from '@ant-design/icons'
import { useTranslation } from 'react-i18next'
import { useNavigate, useParams } from 'react-router-dom'
import { env } from '@/env'
import { EmptyState } from '@/shared/ui/EmptyState'
import { useTencentMap } from '@/shared/map/useTencentMap'
import { TrajectoryLayer } from '@/shared/map/TrajectoryLayer'
import { ProblemMarkerLayer } from '@/shared/map/ProblemMarkerLayer'
import { MapInfoWindow } from '@/shared/map/MapInfoWindow'
import type { InfoWindowLabels, InfoWindowTarget } from '@/shared/map/MapInfoWindow'
import { asLngLat } from '@/shared/map/toLatLng'
import type { ProblemFeatureProperties } from '@/shared/api/types'
import { PanoramaModal } from '@/features/panorama/PanoramaModal'
import type { PanoramaContext } from '@/features/panorama/PanoramaModal'
import { useInspection, useTrajectory } from './api'
import { useProblemsMap } from '@/features/problems/api'

const { Title } = Typography

/** 地图容器固定高度（§10.1 CLS）：减去顶部信息条的视口高度。 */
const MAP_HEIGHT = 'min(72vh, 720px)'

export function InspectionMapPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { id } = useParams<{ id: string }>()
  const inspectionId = Number(id)

  const { data: inspection } = useInspection(inspectionId)
  const { data: trajectory, isLoading: trajectoryLoading } = useTrajectory(inspectionId)
  const { data: problems } = useProblemsMap({ inspection_id: inspectionId }, inspectionId > 0)

  const { containerRef, map, TMap, status, error } = useTencentMap({ apiKey: env.VITE_MAP_KEY })
  const [selected, setSelected] = useState<InfoWindowTarget | null>(null)
  // 「看全景」打开的媒体 + 关联上下文（P5）。null = 弹层关闭。
  const [panorama, setPanorama] = useState<{ mediaId: number; context: PanoramaContext } | null>(
    null,
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

  const routeEmpty =
    !trajectory?.route || (trajectory.route.coordinates?.filter((c) => asLngLat(c)).length ?? 0) < 2
  const problemCount = problems?.features.length ?? 0

  function handleViewPanorama(props: ProblemFeatureProperties): void {
    // P5：用问题封面媒体（cover_media_id 应为 web tier）打开全景弹层，携带关联上下文。
    if (props.cover_media_id == null) {
      navigate(`/panorama?problem_id=${props.id}`)
      return
    }
    setPanorama({
      mediaId: props.cover_media_id,
      context: {
        inspectionStartedAt: inspection?.started_at ?? null,
        problemTypeItemId: props.type_item_id ?? null,
        problemStatusItemId: props.status_item_id ?? null,
        problemTitle: props.title ?? null,
      },
    })
  }
  function handleViewDetail(props: ProblemFeatureProperties): void {
    // P6 stub：跳问题详情。
    navigate(`/problems?problem_id=${props.id}`)
  }

  return (
    <div className="app-page">
      <Space style={{ marginBlockEnd: 'var(--space-5)' }} wrap>
        <Button onClick={() => navigate(`/inspections/${inspectionId}`)}>{t('common.back')}</Button>
        <Title level={3} style={{ margin: 0 }}>
          <EnvironmentOutlined style={{ marginInlineEnd: 'var(--space-3)' }} />
          {t('map.title')}
        </Title>
      </Space>

      {status === 'error' ? (
        <Alert
          type="warning"
          showIcon
          message={t('map.loadFailedTitle')}
          description={
            <Space direction="vertical" size={4}>
              <span>{error ?? t('map.loadFailedHint')}</span>
              <span style={{ color: 'var(--color-ink-muted)' }}>{t('map.fallbackHint')}</span>
              <Button
                size="small"
                onClick={() => navigate(`/problems?inspection_id=${inspectionId}`)}
              >
                {t('map.openProblemList')}
              </Button>
            </Space>
          }
        />
      ) : null}

      <div
        className="map-shell"
        style={{ position: 'relative', height: MAP_HEIGHT }}
        role="region"
        aria-label={t('map.title')}
      >
        <div ref={containerRef} className="map-canvas" style={{ width: '100%', height: '100%' }} />

        {status === 'loading' ? (
          <div className="map-overlay" aria-live="polite">
            <Space direction="vertical" align="center" size={8}>
              <Spin />
              <span style={{ color: 'var(--color-ink-muted)' }}>{t('map.loading')}</span>
            </Space>
          </div>
        ) : null}

        {status === 'ready' && routeEmpty && problemCount === 0 && !trajectoryLoading ? (
          <div className="map-overlay">
            <EmptyState
              description={
                inspection?.status === 'IN_PROGRESS' ? t('map.inProgressEmpty') : t('map.empty')
              }
            />
          </div>
        ) : null}

        {status === 'ready' && map && TMap ? (
          <>
            <TrajectoryLayer map={map} TMap={TMap} route={trajectory?.route} />
            <ProblemMarkerLayer map={map} TMap={TMap} data={problems} onSelect={setSelected} />
            <MapInfoWindow
              map={map}
              TMap={TMap}
              target={selected}
              labels={infoLabels}
              onViewPanorama={handleViewPanorama}
              onViewDetail={handleViewDetail}
              onClose={() => setSelected(null)}
            />
          </>
        ) : null}
      </div>

      <p className="map-foot" style={{ color: 'var(--color-ink-muted)', marginBlockStart: 'var(--space-4)' }}>
        {t('map.problemCount', { count: problemCount })}
      </p>

      <PanoramaModal
        open={panorama !== null}
        mediaId={panorama?.mediaId ?? null}
        context={panorama?.context}
        onClose={() => setPanorama(null)}
      />
    </div>
  )
}
