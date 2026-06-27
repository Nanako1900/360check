/**
 * ProblemPointMap（§P6）：问题详情内的小地图，仅标注单个问题点位。
 *
 * - 复用 P4 的 useTencentMap（SDK 注入 + 实例生命周期）。
 * - 坐标经唯一 `wgsToTMapLatLng`（WGS84→GCJ-02→LatLng(lat,lng)）转换，绝不内联坐标换算（§4.3）。
 * - SDK 加载失败优雅降级：给出坐标文本，地图非唯一查看路径（§10.2）。
 */
import { useEffect, useMemo, useRef } from 'react'
import { Alert, Spin } from 'antd'
import { useTranslation } from 'react-i18next'
import { env } from '@/env'
import { useTencentMap } from '@/shared/map/useTencentMap'
import { asLngLat, wgsToTMapLatLng } from '@/shared/map/toLatLng'
import type { MultiMarker } from '@/shared/map/tmap'
import type { GeoJSONPoint } from '@/shared/api/types'

interface ProblemPointMapProps {
  /** 问题点位（WGS84 GeoJSON Point）。 */
  geom: GeoJSONPoint | undefined
}

const MARKER_STYLE_ID = 'problem-point'
const MAP_HEIGHT = 220

export function ProblemPointMap({ geom }: ProblemPointMapProps) {
  const { t } = useTranslation()
  // 稳定 position 引用：asLngLat 每次返回新数组，未 memo 会让标注 effect 每次父级重渲染都销毁重建。
  const position = useMemo(() => asLngLat(geom?.coordinates ?? []), [geom])
  const { containerRef, map, TMap, status, error } = useTencentMap({
    apiKey: env.VITE_MAP_KEY,
    center: position ?? undefined,
    zoom: 16,
  })
  const markerRef = useRef<MultiMarker | null>(null)

  useEffect(() => {
    if (status !== 'ready' || !map || !TMap || !position) return
    const marker = new TMap.MultiMarker({
      map,
      styles: { [MARKER_STYLE_ID]: { width: 24, height: 32, anchor: { x: 12, y: 32 } } },
      geometries: [
        {
          id: 'p',
          styleId: MARKER_STYLE_ID,
          position: wgsToTMapLatLng(position, TMap),
        },
      ],
    })
    markerRef.current = marker
    return () => {
      marker.setMap(null)
      marker.destroy?.()
      markerRef.current = null
    }
  }, [status, map, TMap, position])

  if (!position) {
    return <Alert type="info" showIcon banner message={t('problem.mapNoPoint')} />
  }

  return (
    <div
      className="map-shell"
      style={{ position: 'relative', height: MAP_HEIGHT, borderRadius: 8, overflow: 'hidden' }}
      role="region"
      aria-label={t('problem.mapRegion')}
    >
      <div ref={containerRef} className="map-canvas" style={{ width: '100%', height: '100%' }} />
      {status === 'loading' ? (
        <div
          className="map-overlay"
          aria-live="polite"
          style={{ display: 'grid', placeItems: 'center' }}
        >
          <Spin />
        </div>
      ) : null}
      {status === 'error' ? (
        <div className="map-overlay" style={{ padding: 'var(--space-3, 12px)' }}>
          <Alert
            type="warning"
            showIcon
            message={t('problem.mapLoadFailed')}
            description={
              <span className="tabular">{`${position[1].toFixed(6)}, ${position[0].toFixed(6)}`}</span>
            }
          />
          {error ? null : null}
        </div>
      ) : null}
    </div>
  )
}
